using System.Net;
using System.Net.Http.Headers;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace SleepySource;

internal sealed class HostedKickState
{
    public string ApiBase { get; set; } = SleepySourceApiService.ApiBase;
    public string Status { get; set; } = "disconnected";
    public bool Connected { get; set; }
    [JsonPropertyName("oauth_pending")]
    public bool OAuthPending { get; set; }
    public string KickUserID { get; set; } = "";
    public string KickUsername { get; set; } = "";
    public List<string> Scopes { get; set; } = [];
    public long TokenExpiresAt { get; set; }
    public string EventsStatus { get; set; } = "inactive";
    public bool EventsReady { get; set; }
    public string RealtimeStatus { get; set; } = "disconnected";
    public int RealtimeClients { get; set; }
    public bool FallbackQueue { get; set; } = true;
    public string LastError { get; set; } = "";
    public long LastEventAt { get; set; }
    public string LastEventType { get; set; } = "";
    public long DeliveredEvents { get; set; }
    public bool CredentialStored { get; set; }
    public string CredentialStorage { get; set; } = "";
}

internal sealed class SleepySourceApiService : IDisposable
{
    public const string ApiBase = "https://sleepysource-api.sleepyservices.workers.dev";
    private const string ConnectionFile = "kick_connection.json";
    private readonly object gate = new();
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(20) };
    private readonly string credentialPath;
    private readonly ChatService chat;
    private readonly AlertService alerts;
    private readonly CancellationTokenSource lifetime = new();
    private CancellationTokenSource? delivery;
    private Task? websocketTask;
    private Task? pollTask;
    private string connectionID = "";
    private string connectionToken = "";
    private string pendingSessionID = "";
    private string pendingPollToken = "";
    private DateTime pendingExpiresAt;
    private HostedKickState state = new();
    private readonly Dictionary<string, long> processed = new(StringComparer.Ordinal);

    public SleepySourceApiService(string dataDir, ChatService chat, AlertService alerts)
    {
        this.chat = chat;
        this.alerts = alerts;
        credentialPath = Path.Combine(dataDir, ConnectionFile);
        http.DefaultRequestHeaders.UserAgent.ParseAdd("SleepySource/1.0.0");
        http.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
        LoadCredential();
        DeleteLegacyCredentialFiles(dataDir);
        lock (gate)
        {
            state.CredentialStored = connectionID.Length > 0 && connectionToken.Length > 0;
            state.CredentialStorage = OperatingSystem.IsWindows() ? "Windows encrypted SleepySource connection" : "local SleepySource connection";
            if (state.CredentialStored) state.Status = "checking";
        }
    }

    public HostedKickState State()
    {
        lock (gate) return Clone(state);
    }

    public void Start()
    {
        if (!HasCredential()) return;
        _ = Task.Run(async () =>
        {
            try
            {
                var ok = await RefreshConnectionAsync(lifetime.Token);
                if (ok)
                {
                    await SyncEventsAsync(lifetime.Token);
                    StartDeliveryLoops();
                }
            }
            catch (OperationCanceledException) { }
            catch (Exception ex) { SetError(ex.Message); }
        });
    }

    public async Task<bool> CheckApiHealthAsync(CancellationToken ct)
    {
        try
        {
            using var response = await http.GetAsync(ApiBase + "/health", ct);
            return response.IsSuccessStatusCode;
        }
        catch { return false; }
    }

    public async Task<HostedKickState> BeginOAuthAsync(CancellationToken ct)
    {
        using var response = await http.PostAsync(ApiBase + "/oauth/kick/start", new StringContent("{}", Encoding.UTF8, "application/json"), ct);
        var bytes = await response.Content.ReadAsByteArrayAsync(ct);
        if (!response.IsSuccessStatusCode) throw ApiException(bytes, response.StatusCode, "Could not start Kick authorization");
        using var doc = JsonDocument.Parse(bytes);
        var root = doc.RootElement;
        var session = String(root, "session_id");
        var poll = String(root, "poll_token");
        var authorize = String(root, "authorize_url");
        var expires = Int64(root, "expires_in", 600);
        if (session.Length == 0 || poll.Length == 0 || authorize.Length == 0) throw new InvalidOperationException("SleepySource API returned an incomplete authorization session");
        lock (gate)
        {
            pendingSessionID = session;
            pendingPollToken = poll;
            pendingExpiresAt = DateTime.UtcNow.AddSeconds(Math.Max(60, expires));
            state.OAuthPending = true;
            state.Status = "authorizing";
            state.LastError = "";
        }
        AppUtil.OpenExternal(authorize);
        return State();
    }

    public async Task<HostedKickState> PollOAuthAsync(CancellationToken ct)
    {
        string session, poll;
        DateTime expires;
        lock (gate) { session = pendingSessionID; poll = pendingPollToken; expires = pendingExpiresAt; }
        if (session.Length == 0 || poll.Length == 0) return State();
        if (expires != default && DateTime.UtcNow > expires)
        {
            ClearPending("Kick authorization expired. Choose Connect with Kick and try again.");
            return State();
        }

        var payload = JsonSerializer.SerializeToUtf8Bytes(new { session_id = session, poll_token = poll }, AppUtil.Json);
        using var content = new ByteArrayContent(payload);
        content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
        using var response = await http.PostAsync(ApiBase + "/oauth/kick/status", content, ct);
        var bytes = await response.Content.ReadAsByteArrayAsync(ct);
        if (!response.IsSuccessStatusCode) throw ApiException(bytes, response.StatusCode, "Could not check Kick authorization");
        using var doc = JsonDocument.Parse(bytes);
        var root = doc.RootElement;
        var remoteStatus = String(root, "status");
        if (remoteStatus is "pending" or "processing")
        {
            lock (gate) { state.OAuthPending = true; state.Status = remoteStatus == "processing" ? "finishing" : "authorizing"; }
            return State();
        }
        if (remoteStatus is "failed" or "disconnected")
        {
            ClearPending("Kick authorization was not completed: " + (String(root, "error") is var err && err.Length > 0 ? err.Replace('_', ' ') : remoteStatus));
            return State();
        }
        if (remoteStatus != "completed" || !root.TryGetProperty("connection", out var connection))
        {
            ClearPending("SleepySource API returned an unexpected authorization status");
            return State();
        }

        var id = String(connection, "connection_id");
        if (id.Length == 0) throw new InvalidOperationException("SleepySource API did not return a connection ID");
        lock (gate)
        {
            connectionID = id;
            connectionToken = poll;
            pendingSessionID = pendingPollToken = "";
            pendingExpiresAt = default;
            state.OAuthPending = false;
            ApplyPublicConnectionLocked(connection);
            state.Status = "connected";
            state.Connected = true;
            state.CredentialStored = true;
            state.LastError = "";
        }
        await SaveCredentialAsync();
        ApplyChatConnection();
        await SyncEventsAsync(ct);
        StartDeliveryLoops();
        return State();
    }

    public async Task<bool> RefreshConnectionAsync(CancellationToken ct)
    {
        if (!HasCredential()) { SetDisconnectedState(); return false; }
        using var result = await PostAuthenticatedAsync("/kick/connection/status", null, ct, allowNonSuccess: true);
        var status = String(result.RootElement, "status");
        if (status == "connected")
        {
            if (result.RootElement.TryGetProperty("connection", out var connection))
            {
                lock (gate)
                {
                    ApplyPublicConnectionLocked(connection);
                    state.Connected = true;
                    state.Status = "connected";
                    state.LastError = "";
                }
                ApplyChatConnection();
            }
            return true;
        }
        if (status == "refreshing")
        {
            lock (gate) { state.Status = "refreshing"; state.LastError = ""; }
            return true;
        }
        if (status is "disconnected" or "reconnect_required")
        {
            lock (gate) { state.Connected = false; state.Status = "reconnect_required"; state.LastError = "Reconnect Kick to continue using Kick-powered features."; }
            chat.ClearHostedConnection();
            StopDeliveryLoops();
            return false;
        }
        lock (gate)
        {
            state.Status = status.Length > 0 ? status : "degraded";
            state.LastError = String(result.RootElement, "error").Replace('_', ' ');
        }
        return false;
    }

    public async Task<HostedKickState> SyncEventsAsync(CancellationToken ct)
    {
        if (!HasCredential()) return State();
        using var result = await PostAuthenticatedAsync("/kick/events/sync", null, ct, allowNonSuccess: true);
        var status = String(result.RootElement, "status");
        var ready = Bool(result.RootElement, "all_required_subscribed");
        lock (gate)
        {
            state.EventsStatus = status.Length > 0 ? status : (ready ? "subscribed" : "partial");
            state.EventsReady = ready;
            if (!ready)
            {
                var missing = new List<string>();
                if (result.RootElement.TryGetProperty("missing_events", out var arr) && arr.ValueKind == JsonValueKind.Array)
                    foreach (var item in arr.EnumerateArray()) { var name = String(item, "name"); if (name.Length > 0) missing.Add(name); }
                state.LastError = missing.Count > 0 ? "Missing Kick event subscriptions: " + string.Join(", ", missing) : String(result.RootElement, "error").Replace('_', ' ');
            }
        }
        chat.SetHostedEventsState(ready, ready ? "Managed Kick event subscriptions active" : "Kick event subscriptions need attention", ready ? "" : State().LastError);
        return State();
    }

    public async Task<HostedKickState> DisconnectAsync(CancellationToken ct)
    {
        if (HasCredential())
        {
            using var result = await PostAuthenticatedAsync("/kick/connection/disconnect", null, ct, allowNonSuccess: true);
            var status = String(result.RootElement, "status");
            if (status != "disconnected")
            {
                var err = String(result.RootElement, "error");
                throw new InvalidOperationException(err.Length > 0 ? "Kick disconnect failed: " + err.Replace('_', ' ') : "Kick disconnect failed. Try again.");
            }
        }
        ClearLocalCredential();
        return State();
    }

    public async Task<JsonElement> StreamMetadataAsync(CancellationToken ct)
    {
        using var doc = await PostAuthenticatedAsync("/kick/channel/metadata", null, ct);
        return doc.RootElement.Clone();
    }

    public async Task<JsonElement> SearchCategoriesAsync(string query, CancellationToken ct)
    {
        using var doc = await PostAuthenticatedAsync("/kick/categories/search", new Dictionary<string, object?> { ["query"] = (query ?? "").Trim() }, ct);
        return doc.RootElement.Clone();
    }

    public async Task<JsonElement> UpdateStreamAsync(string title, long categoryID, string categoryName, CancellationToken ct)
    {
        using var doc = await PostAuthenticatedAsync("/kick/channel/update", new Dictionary<string, object?>
        {
            ["title"] = (title ?? "").Trim(), ["category_id"] = categoryID, ["category_name"] = (categoryName ?? "").Trim()
        }, ct);
        return doc.RootElement.Clone();
    }

    private void StartDeliveryLoops()
    {
        if (!HasCredential()) return;
        lock (gate)
        {
            if (delivery is not null && !delivery.IsCancellationRequested) return;
            delivery = CancellationTokenSource.CreateLinkedTokenSource(lifetime.Token);
            websocketTask = Task.Run(() => WebSocketLoopAsync(delivery.Token));
            pollTask = Task.Run(() => PollLoopAsync(delivery.Token));
        }
    }

    private void StopDeliveryLoops()
    {
        CancellationTokenSource? old;
        lock (gate) { old = delivery; delivery = null; websocketTask = pollTask = null; state.RealtimeStatus = "disconnected"; state.RealtimeClients = 0; }
        try { old?.Cancel(); } catch { }
        try { old?.Dispose(); } catch { }
        chat.SetRealtimeState(false, "Hosted Kick realtime disconnected");
    }

    private async Task WebSocketLoopAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested && HasCredential())
        {
            try
            {
                string id, token;
                lock (gate) { id = connectionID; token = connectionToken; state.RealtimeStatus = "connecting"; }
                using var ws = new ClientWebSocket();
                ws.Options.SetRequestHeader("X-SleepySource-Connection-Id", id);
                ws.Options.SetRequestHeader("X-SleepySource-Connection-Token", token);
                var uri = new Uri(ApiBase.Replace("https://", "wss://", StringComparison.OrdinalIgnoreCase).Replace("http://", "ws://", StringComparison.OrdinalIgnoreCase) + "/realtime/connect");
                await ws.ConnectAsync(uri, ct);
                lock (gate) { state.RealtimeStatus = "connected"; state.RealtimeClients = 1; state.LastError = ""; }
                chat.SetRealtimeState(true, "Hosted Kick realtime connected");
                while (!ct.IsCancellationRequested && ws.State == WebSocketState.Open)
                {
                    var text = await ReceiveTextAsync(ws, ct);
                    if (text is null) break;
                    if (text == "pong") continue;
                    using var doc = JsonDocument.Parse(text);
                    var root = doc.RootElement;
                    if (String(root, "type") == "ready") continue;
                    if (String(root, "type") == "kick_event" && root.TryGetProperty("event", out var ev))
                    {
                        var messageID = String(ev, "message_id");
                        if (ProcessEvent(ev) && messageID.Length > 0) await AckAsync([messageID], ct);
                    }
                }
            }
            catch (OperationCanceledException) { break; }
            catch (Exception ex)
            {
                lock (gate) { state.RealtimeStatus = "reconnecting"; state.RealtimeClients = 0; state.LastError = "Realtime: " + ex.Message; }
                chat.SetRealtimeState(false, "Hosted Kick realtime reconnecting");
            }
            if (!ct.IsCancellationRequested) try { await Task.Delay(3000, ct); } catch { }
        }
    }

    private async Task PollLoopAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested && HasCredential())
        {
            try
            {
                using var doc = await PostAuthenticatedAsync("/kick/events/delivery/poll", new Dictionary<string, object?> { ["limit"] = 10 }, ct, allowNonSuccess: true);
                var status = String(doc.RootElement, "status");
                if (status == "disconnected" || status == "reconnect_required")
                {
                    lock (gate) { state.Connected = false; state.Status = "reconnect_required"; }
                    chat.ClearHostedConnection();
                    break;
                }
                var ack = new List<string>();
                if (doc.RootElement.TryGetProperty("events", out var events) && events.ValueKind == JsonValueKind.Array)
                {
                    foreach (var ev in events.EnumerateArray())
                    {
                        var messageID = String(ev, "message_id");
                        if (ProcessEvent(ev) && messageID.Length > 0) ack.Add(messageID);
                    }
                }
                if (ack.Count > 0) await AckAsync(ack, ct);
                var retry = (int)Math.Clamp(Int64(doc.RootElement, "retry_after_ms", ack.Count > 0 ? 100 : 1000), 100, 5000);
                await Task.Delay(retry, ct);
            }
            catch (OperationCanceledException) { break; }
            catch (Exception ex)
            {
                lock (gate) { if (state.RealtimeStatus != "connected") state.LastError = "Event delivery: " + ex.Message; }
                try { await Task.Delay(2000, ct); } catch { }
            }
        }
    }

    private bool ProcessEvent(JsonElement envelope)
    {
        var messageID = String(envelope, "message_id");
        var type = String(envelope, "event_type");
        if (messageID.Length == 0 || type.Length == 0 || !envelope.TryGetProperty("payload", out var payload)) return false;
        lock (processed)
        {
            var now = AppUtil.NowMS();
            foreach (var key in processed.Where(x => x.Value < now - 86_400_000).Select(x => x.Key).ToList()) processed.Remove(key);
            if (processed.ContainsKey(messageID)) return true;
            processed[messageID] = now;
        }
        try
        {
            if (type.Equals("chat.message.sent", StringComparison.OrdinalIgnoreCase))
            {
                var msg = ChatService.ParseKickChat(payload) ?? throw new InvalidDataException("invalid Kick chat payload");
                chat.AddMessage(msg);
            }
            else if (KickAlertParser.IsSupported(type))
            {
                var body = JsonSerializer.SerializeToUtf8Bytes(payload, AppUtil.Json);
                var parsed = KickAlertParser.Parse(type, messageID, body, chat.State().BroadcasterUserID);
                alerts.Enqueue(parsed.Event, parsed.Dedupe);
            }
            chat.MarkHostedEvent(type, true);
            lock (gate) { state.LastEventAt = AppUtil.NowMS(); state.LastEventType = type; state.DeliveredEvents++; state.LastError = ""; }
            return true;
        }
        catch (Exception ex)
        {
            lock (processed) processed.Remove(messageID);
            chat.MarkHostedEvent(type, false, ex.Message);
            lock (gate) state.LastError = "Event processing: " + ex.Message;
            return false;
        }
    }

    private async Task AckAsync(IReadOnlyCollection<string> messageIDs, CancellationToken ct)
    {
        if (messageIDs.Count == 0) return;
        using var _ = await PostAuthenticatedAsync("/kick/events/delivery/ack", new Dictionary<string, object?> { ["message_ids"] = messageIDs.ToArray() }, ct, allowNonSuccess: true);
    }

    private async Task<JsonDocument> PostAuthenticatedAsync(string path, IDictionary<string, object?>? extra, CancellationToken ct, bool allowNonSuccess = false)
    {
        string id, token;
        lock (gate) { id = connectionID; token = connectionToken; }
        if (id.Length == 0 || token.Length == 0) throw new UnauthorizedAccessException("Connect with Kick first");
        var body = new Dictionary<string, object?>(StringComparer.Ordinal) { ["connection_id"] = id, ["connection_token"] = token };
        if (extra is not null) foreach (var pair in extra) body[pair.Key] = pair.Value;
        var payload = JsonSerializer.SerializeToUtf8Bytes(body, AppUtil.Json);
        using var request = new HttpRequestMessage(HttpMethod.Post, ApiBase + path) { Content = new ByteArrayContent(payload) };
        request.Content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
        using var response = await http.SendAsync(request, ct);
        var bytes = await response.Content.ReadAsByteArrayAsync(ct);
        JsonDocument doc;
        try { doc = JsonDocument.Parse(bytes.Length == 0 ? "{}"u8.ToArray() : bytes); }
        catch { throw new InvalidOperationException("SleepySource API returned an invalid response"); }
        if (!response.IsSuccessStatusCode && !allowNonSuccess)
        {
            var ex = ApiException(bytes, response.StatusCode, "SleepySource API request failed");
            doc.Dispose();
            throw ex;
        }
        return doc;
    }

    private static async Task<string?> ReceiveTextAsync(ClientWebSocket ws, CancellationToken ct)
    {
        var buffer = new byte[16 * 1024];
        using var ms = new MemoryStream();
        while (true)
        {
            var result = await ws.ReceiveAsync(new ArraySegment<byte>(buffer), ct);
            if (result.MessageType == WebSocketMessageType.Close) return null;
            if (result.MessageType != WebSocketMessageType.Text) continue;
            ms.Write(buffer, 0, result.Count);
            if (ms.Length > 2L << 20) throw new InvalidDataException("Realtime event is too large");
            if (result.EndOfMessage) return Encoding.UTF8.GetString(ms.ToArray());
        }
    }

    private void ApplyChatConnection()
    {
        HostedKickState current = State();
        if (current.Connected) chat.SetHostedConnection(current.KickUsername, current.KickUserID);
    }

    private bool HasCredential() { lock (gate) return connectionID.Length > 0 && connectionToken.Length > 0; }

    private void ApplyPublicConnectionLocked(JsonElement connection)
    {
        state.KickUserID = String(connection, "kick_user_id");
        state.KickUsername = AppUtil.NormalizeKickChannelSlug(String(connection, "kick_username"));
        state.TokenExpiresAt = Int64(connection, "token_expires_at", 0);
        state.Scopes = [];
        if (connection.TryGetProperty("scopes", out var scopes) && scopes.ValueKind == JsonValueKind.Array)
            foreach (var scope in scopes.EnumerateArray()) { var value = scope.GetString()?.Trim() ?? ""; if (value.Length > 0) state.Scopes.Add(value); }
    }

    private void LoadCredential()
    {
        if (!File.Exists(credentialPath)) return;
        try
        {
            using var doc = JsonDocument.Parse(File.ReadAllBytes(credentialPath));
            var root = doc.RootElement;
            var encoded = String(root, "protected_data");
            if (encoded.Length == 0) return;
            var raw = AppUtil.UnprotectCredential(Convert.FromBase64String(encoded));
            using var secret = JsonDocument.Parse(raw);
            connectionID = String(secret.RootElement, "connection_id");
            connectionToken = String(secret.RootElement, "connection_token");
        }
        catch { connectionID = connectionToken = ""; }
    }

    private async Task SaveCredentialAsync()
    {
        string id, token;
        lock (gate) { id = connectionID; token = connectionToken; }
        if (id.Length == 0 || token.Length == 0) return;
        var raw = JsonSerializer.SerializeToUtf8Bytes(new { connection_id = id, connection_token = token }, AppUtil.Json);
        var protectedData = AppUtil.ProtectCredential(raw);
        await AppUtil.AtomicWriteJsonAsync(credentialPath, new { version = 2, protected_data = Convert.ToBase64String(protectedData) });
    }

    private void ClearLocalCredential()
    {
        StopDeliveryLoops();
        lock (gate)
        {
            connectionID = connectionToken = pendingSessionID = pendingPollToken = "";
            pendingExpiresAt = default;
            state = new HostedKickState { Status = "disconnected", CredentialStorage = OperatingSystem.IsWindows() ? "Windows encrypted SleepySource connection" : "local SleepySource connection" };
        }
        try { File.Delete(credentialPath); } catch { }
        chat.ClearHostedConnection();
    }

    private void SetDisconnectedState()
    {
        lock (gate) { state.Connected = false; state.Status = "disconnected"; state.OAuthPending = false; state.RealtimeStatus = "disconnected"; state.EventsStatus = "inactive"; state.EventsReady = false; }
        chat.ClearHostedConnection();
    }

    private void ClearPending(string error)
    {
        lock (gate)
        {
            pendingSessionID = pendingPollToken = ""; pendingExpiresAt = default;
            state.OAuthPending = false;
            state.Status = state.Connected ? "connected" : "disconnected";
            state.LastError = error;
        }
    }

    private void SetError(string error)
    {
        lock (gate) { state.LastError = (error ?? "").Trim(); if (!state.Connected) state.Status = "degraded"; }
    }

    private static void DeleteLegacyCredentialFiles(string dataDir)
    {
        foreach (var name in new[] { "kick_credentials.json", "kick_user_authorization.json" })
            try { File.Delete(Path.Combine(dataDir, name)); } catch { }
    }

    private static Exception ApiException(byte[] bytes, HttpStatusCode code, string fallback)
    {
        string message = "";
        try
        {
            using var doc = JsonDocument.Parse(bytes);
            message = String(doc.RootElement, "kick_message");
            if (message.Length == 0) message = String(doc.RootElement, "error").Replace('_', ' ');
        }
        catch { }
        if (message.Length == 0) message = fallback + " (HTTP " + (int)code + ")";
        return code is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden ? new UnauthorizedAccessException(message) : new InvalidOperationException(message);
    }

    private static string String(JsonElement root, string name) => root.ValueKind == JsonValueKind.Object && root.TryGetProperty(name, out var value) ? (value.ValueKind == JsonValueKind.String ? value.GetString()?.Trim() ?? "" : value.ToString().Trim()) : "";
    private static long Int64(JsonElement root, string name, long fallback) => root.ValueKind == JsonValueKind.Object && root.TryGetProperty(name, out var value) && (value.TryGetInt64(out var number) || long.TryParse(value.ToString(), out number)) ? number : fallback;
    private static bool Bool(JsonElement root, string name) => root.ValueKind == JsonValueKind.Object && root.TryGetProperty(name, out var value) && (value.ValueKind == JsonValueKind.True || (value.ValueKind == JsonValueKind.String && bool.TryParse(value.GetString(), out var parsed) && parsed));
    private static HostedKickState Clone(HostedKickState value) => JsonSerializer.Deserialize<HostedKickState>(JsonSerializer.Serialize(value, AppUtil.Json), AppUtil.Json)!;

    public void Stop()
    {
        StopDeliveryLoops();
    }

    public void Dispose()
    {
        try { lifetime.Cancel(); } catch { }
        StopDeliveryLoops();
        lifetime.Dispose();
        http.Dispose();
    }
}
