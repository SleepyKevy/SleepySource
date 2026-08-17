using System.Text;
using System.Text.Json;

namespace SleepySource;

internal sealed class ChatService
{
    private readonly object gate = new();
    private readonly SemaphoreSlim authGate = new(1, 1);
    private readonly string settingsPath;
    private readonly string credentialsPath;
    private ChatSettings settings;
    private readonly List<ChatMessage> messages = [];
    private string clientID = "";
    private string clientSecret = "";
    private string accessToken = "";
    private DateTime accessTokenExpiresAt;
    private string authMode = "";
    private string connectedChannel = "";
    private string broadcasterUserID = "";
    private string chatroomID = "";
    private bool liveChatConnected;
    private string liveChatStatus = "";
    private bool webhookSubscribed;
    private string webhookSubscriptionID = "";
    private long webhookLastEventAt;
    private long webhookRequestCount;
    private long webhookVerifiedCount;
    private long webhookAcceptedCount;
    private long webhookRejectedCount;
    private long webhookLastRequestAt;
    private string webhookLastEventType = "";
    private string webhookLastError = "";
    private readonly Dictionary<string, long> webhookSeen = new(StringComparer.Ordinal);
    private long updatedAt = AppUtil.NowMS();

    public ChatService(string dataDir)
    {
        settingsPath = Path.Combine(dataDir, "chat_settings.json");
        credentialsPath = Path.Combine(dataDir, "kick_credentials.json");
        settings = AppUtil.LoadJsonOrDefault(settingsPath, () => new ChatSettings(), "chat_settings");
        Normalize(settings);
        LoadSavedCredentials();
        try { SaveSettingsAsync().GetAwaiter().GetResult(); } catch { }
    }

    public void ReloadFromDisk()
    {
        var next = AppUtil.LoadJsonOrDefault(settingsPath, () => new ChatSettings(), "chat settings");
        Normalize(next);
        lock (gate) { settings = next; messages.Clear(); webhookSeen.Clear(); webhookLastError = ""; updatedAt = AppUtil.NowMS(); }
        LoadSavedCredentials();
    }

    public ChatState State()
    {
        lock (gate)
        {
            var valid = !string.IsNullOrWhiteSpace(accessToken) && (accessTokenExpiresAt == default || DateTime.UtcNow < accessTokenExpiresAt);
            var ready = valid || (authMode == "app" && clientID.Length > 0 && clientSecret.Length > 0);
            return new ChatState
            {
                Settings = Clone(settings), Messages = messages.Select(Clone).ToList(), UpdatedAt = updatedAt, AuthReady = ready, AuthMode = authMode,
                ConnectedChannel = connectedChannel, BroadcasterUserID = broadcasterUserID,
                TokenExpiresAt = accessTokenExpiresAt == default ? 0 : new DateTimeOffset(accessTokenExpiresAt).ToUnixTimeSeconds(),
                LiveChatConnected = liveChatConnected, LiveChatStatus = liveChatStatus, ChatroomID = chatroomID,
                WebhookSubscribed = webhookSubscribed, WebhookSubscriptionID = webhookSubscriptionID, WebhookLastEventAt = webhookLastEventAt,
                WebhookRequestCount = webhookRequestCount, WebhookVerifiedCount = webhookVerifiedCount, WebhookAcceptedCount = webhookAcceptedCount,
                WebhookRejectedCount = webhookRejectedCount, WebhookLastRequestAt = webhookLastRequestAt, WebhookLastEventType = webhookLastEventType,
                WebhookLastError = webhookLastError, SavedClientID = clientID,
                CredentialsSaved = settings.RememberKickLogin && clientID.Length > 0 && clientSecret.Length > 0 && OperatingSystem.IsWindows(),
                CredentialStorage = OperatingSystem.IsWindows() ? "Windows encrypted storage" : "memory only"
            };
        }
    }

    public async Task SetSettingsAsync(ChatSettings next)
    {
        Normalize(next); string cid, secret;
        lock (gate) { settings = next; updatedAt = AppUtil.NowMS(); cid = clientID; secret = clientSecret; }
        await SaveSettingsAsync(); await SaveCredentialsAsync(cid, secret, next.RememberKickLogin);
    }

    public void AddMessage(ChatMessage msg)
    {
        lock (gate)
        {
            msg.ID = (msg.ID ?? "").Trim(); if (msg.ID.Length == 0) msg.ID = "local-" + DateTime.UtcNow.Ticks; if (msg.ID.Length > 128) msg.ID = msg.ID[..128];
            msg.UserID = (msg.UserID ?? "").Trim(); if (msg.UserID.Length > 64) msg.UserID = msg.UserID[..64];
            msg.Username = (msg.Username ?? "").Trim(); if (msg.Username.Length == 0) msg.Username = "Viewer"; if (msg.Username.Length > 80) msg.Username = msg.Username[..80];
            msg.Text = (msg.Text ?? "").Trim(); if (msg.Text.Length > 4000) msg.Text = msg.Text[..4000];
            msg.AvatarURL = (msg.AvatarURL ?? "").Trim(); if (msg.AvatarURL.Length > 2048) msg.AvatarURL = "";
            msg.Badges ??= []; if (msg.Badges.Count > 12) msg.Badges = msg.Badges.Take(12).ToList(); for (var i=0;i<msg.Badges.Count;i++) { msg.Badges[i] = msg.Badges[i].Trim(); if (msg.Badges[i].Length > 48) msg.Badges[i] = msg.Badges[i][..48]; }
            msg.BadgeDetails ??= []; if (msg.BadgeDetails.Count > 12) msg.BadgeDetails = msg.BadgeDetails.Take(12).ToList(); foreach (var b in msg.BadgeDetails) { b.Text = (b.Text ?? "").Trim(); if (b.Text.Length > 64) b.Text = b.Text[..64]; b.Type = NormalizeBadgeType(b.Type); b.Count = Math.Max(0, b.Count); }
            if (msg.CreatedAt <= 0) msg.CreatedAt = AppUtil.NowMS(); msg.Color = AppUtil.NormalizeColor(msg.Color, "#55B7FF");
            if (settings.HideCommands && msg.Text.StartsWith('!')) return;
            if (messages.TakeLast(100).Any(x => x.ID == msg.ID)) return;
            messages.Add(Clone(msg)); var keep = Math.Max(30, Math.Max(1, settings.MaxMessages) * 3); if (messages.Count > keep) messages.RemoveRange(0, messages.Count - keep); updatedAt = AppUtil.NowMS();
        }
    }

    public void ClearMessages() { lock (gate) { messages.Clear(); updatedAt = AppUtil.NowMS(); } }
    public void SetLegacyAccessToken(string token) { lock (gate) { clientID = clientSecret = ""; accessToken = token.Trim(); accessTokenExpiresAt = default; webhookSubscribed = false; webhookSubscriptionID = ""; webhookLastEventAt = 0; authMode = accessToken.Length > 0 ? "legacy-user-token" : ""; if (accessToken.Length == 0) { connectedChannel = broadcasterUserID = chatroomID = ""; } updatedAt = AppUtil.NowMS(); } }
    public async Task SetAppCredentialsAsync(string id, string secret) { id = id.Trim(); secret = secret.Trim(); bool remember; lock (gate) { clientID = id; clientSecret = secret; authMode = id.Length > 0 && secret.Length > 0 ? "app" : ""; updatedAt = AppUtil.NowMS(); remember = settings.RememberKickLogin; } await SaveCredentialsAsync(id, secret, remember); }
    public void SetKickAppToken(string token, DateTime expiresAt) { lock (gate) { accessToken = token.Trim(); accessTokenExpiresAt = expiresAt; if (clientID.Length > 0 && clientSecret.Length > 0) authMode = "app"; updatedAt = AppUtil.NowMS(); } }
    public void DisconnectAuth() { lock (gate) { accessToken = ""; accessTokenExpiresAt = default; connectedChannel = broadcasterUserID = chatroomID = ""; webhookSubscribed = false; webhookSubscriptionID = ""; webhookLastEventAt = 0; liveChatConnected = false; liveChatStatus = ""; authMode = clientID.Length > 0 && clientSecret.Length > 0 ? "app" : ""; updatedAt = AppUtil.NowMS(); } }
    public void ClearAuth() { lock (gate) { clientID = clientSecret = accessToken = authMode = connectedChannel = broadcasterUserID = chatroomID = webhookSubscriptionID = liveChatStatus = ""; accessTokenExpiresAt = default; webhookSubscribed = false; webhookLastEventAt = 0; liveChatConnected = false; updatedAt = AppUtil.NowMS(); } try { File.Delete(credentialsPath); } catch { } }
    public bool HasAppCredentials() { lock (gate) return clientID.Length > 0 && clientSecret.Length > 0; }
    public (string ClientID, string ClientSecret, bool OK) AppCredentials() { lock (gate) return (clientID, clientSecret, clientID.Length > 0 && clientSecret.Length > 0); }
    public void SetResolvedChannel(string channel, string id) { lock (gate) { connectedChannel = AppUtil.NormalizeKickChannelSlug(channel); broadcasterUserID = id.Trim(); settings.KickChannel = connectedChannel; updatedAt = AppUtil.NowMS(); } _ = SaveSettingsAsync(); }
    public void SetWebhookSubscription(string id, string status) { lock (gate) { webhookSubscriptionID = id.Trim(); webhookSubscribed = id.Length > 0; webhookLastError = ""; liveChatStatus = status; updatedAt = AppUtil.NowMS(); } }
    public void MarkWebhookRequest(string type) { lock (gate) { webhookRequestCount++; webhookLastRequestAt = AppUtil.NowMS(); webhookLastEventType = type.Trim(); } }
    public void MarkWebhookVerified(string type) { lock (gate) { webhookVerifiedCount++; webhookLastEventType = type.Trim(); webhookLastError = ""; } }
    public void MarkWebhookRejected(string error) { lock (gate) { webhookRejectedCount++; webhookLastError = error.Trim(); updatedAt = AppUtil.NowMS(); } }
    public void MarkWebhookAccepted(string type, bool chat) { lock (gate) { webhookAcceptedCount++; webhookLastEventAt = AppUtil.NowMS(); webhookLastEventType = type.Trim(); webhookLastError = ""; if (chat) { liveChatConnected = true; liveChatStatus = "Official Kick webhook active"; } updatedAt = AppUtil.NowMS(); } }
    public bool AcceptWebhookMessageID(string id) { id = id.Trim(); if (id.Length == 0) return true; lock (gate) { var now = AppUtil.NowMS(); foreach (var k in webhookSeen.Where(x => x.Value < now - 86_400_000).Select(x => x.Key).ToList()) webhookSeen.Remove(k); if (webhookSeen.ContainsKey(id)) return false; webhookSeen[id] = now; return true; } }

    public async Task<string> EnsureKickAccessTokenAsync(KickService kick, CancellationToken ct)
    {
        await authGate.WaitAsync(ct); try
        {
            string token, cid, secret, mode; DateTime expires;
            lock (gate) { token = accessToken; expires = accessTokenExpiresAt; cid = clientID; secret = clientSecret; mode = authMode; }
            if (token.Length > 0 && (expires == default || DateTime.UtcNow.AddSeconds(45) < expires)) return token;
            if (mode == "legacy-user-token" && token.Length > 0) return token;
            var result = await kick.RequestAppAccessTokenAsync(cid, secret, ct); SetKickAppToken(result.Token, result.ExpiresAt); return result.Token;
        }
        finally { authGate.Release(); }
    }

    public static void Normalize(ChatSettings s)
    {
        s.SchemaVersion = Math.Max(6, s.SchemaVersion); s.CanvasWidth = AppUtil.Clamp(s.CanvasWidth, 320, 3840); s.CanvasHeight = AppUtil.Clamp(s.CanvasHeight, 240, 2160); s.BoxWidth = AppUtil.Clamp(s.BoxWidth, 180, 3840); s.BoxHeight = AppUtil.Clamp(s.BoxHeight, 120, 2160); s.BoxX = AppUtil.Clamp(s.BoxX, -3840, 3840); s.BoxY = AppUtil.Clamp(s.BoxY, -2160, 2160);
        s.FontFamily = string.IsNullOrWhiteSpace(s.FontFamily) ? "Segoe UI" : s.FontFamily.Trim(); s.FontSize = AppUtil.Clamp(s.FontSize, 10, 96); s.UsernameSize = AppUtil.Clamp(s.UsernameSize, 10, 96); s.MessageColor = AppUtil.NormalizeColor(s.MessageColor, "#FFFFFF"); s.UsernameColor = AppUtil.NormalizeColor(s.UsernameColor, "#55B7FF"); s.BackgroundColor = AppUtil.NormalizeColor(s.BackgroundColor, "#07111F"); s.BorderColor = AppUtil.NormalizeColor(s.BorderColor, "#2F78B7"); s.BackgroundOpacity = AppUtil.Clamp(s.BackgroundOpacity, 0, 100); s.BorderWidth = AppUtil.Clamp(s.BorderWidth, 0, 12); s.Radius = AppUtil.Clamp(s.Radius, 0, 100); s.Padding = AppUtil.Clamp(s.Padding, 0, 80); s.MessageGap = AppUtil.Clamp(s.MessageGap, 0, 60); s.EmoteSize = AppUtil.Clamp(s.EmoteSize, 16, 96); s.BadgeSize = AppUtil.Clamp(s.BadgeSize, 12, 64); s.MaxMessages = AppUtil.Clamp(s.MaxMessages, 1, 50); s.MessageBackgroundColor = AppUtil.NormalizeColor(s.MessageBackgroundColor, "#07111F"); s.MessageBackgroundOpacity = AppUtil.Clamp(s.MessageBackgroundOpacity, 0, 100); s.AnimationMS = AppUtil.Clamp(s.AnimationMS, 0, 3000); s.MessageBorderWidth = AppUtil.Clamp(s.MessageBorderWidth, 0, 12); s.MessageBorderColor = AppUtil.NormalizeColor(s.MessageBorderColor, "#2F78B7"); s.MessageRadius = AppUtil.Clamp(s.MessageRadius, 0, 80); s.BoxBlur = AppUtil.Clamp(s.BoxBlur, 0, 40); s.UsernameWeight = AppUtil.Clamp(s.UsernameWeight, 100, 900);
        if (!new[] { "midnight", "glass", "neon", "minimal", "bubblegum", "custom" }.Contains(s.Theme)) s.Theme = "midnight"; if (!new[] { "linear", "ease", "ease-in", "ease-out", "ease-in-out", "snappy", "spring", "smooth" }.Contains(s.AnimationEasing)) s.AnimationEasing = "smooth"; if (s.Direction != "top-down") s.Direction = "bottom-up"; if (!new[] { "fade", "slide-left", "slide-right", "slide-up", "pop", "zoom", "bounce", "blur", "flip", "none" }.Contains(s.Animation)) s.Animation = "slide-up"; s.KickChannel = AppUtil.NormalizeKickChannelSlug(s.KickChannel); s.SevenTVEmoteSetID = (s.SevenTVEmoteSetID ?? "").Trim();
    }

    public static string NormalizeBadgeType(string? raw)
    {
        var v = (raw ?? "").Trim().ToLowerInvariant().Replace('_','-');
        if (v.Contains("broadcaster")) return "broadcaster"; if (v.Contains("moderator") || v == "mod") return "moderator"; if (v.Contains("verified")) return "verified"; if (v.Contains("subscriber") || v.Contains("subscription")) return "subscriber"; if (v.Contains("vip")) return "vip"; if (v.Contains("founder")) return "founder"; if (v.Contains("og")) return "og"; return v.Length > 48 ? v[..48] : v;
    }

    public static ChatMessage? ParseKickChat(JsonElement raw)
    {
        try
        {
            var id = raw.GetProperty("message_id").GetString()?.Trim() ?? ""; var content = raw.GetProperty("content").GetString()?.Trim() ?? ""; if (id.Length == 0 || content.Length == 0) return null;
            var sender = raw.GetProperty("sender"); var msg = new ChatMessage { ID = id, UserID = sender.TryGetProperty("user_id", out var uid) ? uid.GetInt64().ToString() : "", Username = sender.TryGetProperty("username", out var un) ? un.GetString()?.Trim() ?? "" : "", Text = content, AvatarURL = sender.TryGetProperty("profile_picture", out var av) ? av.GetString()?.Trim() ?? "" : "" };
            if (raw.TryGetProperty("created_at", out var ca) && DateTimeOffset.TryParse(ca.GetString(), out var dt)) msg.CreatedAt = dt.ToUnixTimeMilliseconds();
            long broadcaster = 0; if (raw.TryGetProperty("broadcaster", out var br) && br.TryGetProperty("user_id", out var bu)) broadcaster = bu.GetInt64(); long senderID = 0; if (sender.TryGetProperty("user_id", out var su)) senderID = su.GetInt64();
            var types = new HashSet<string>(); if (sender.TryGetProperty("identity", out var ident) && ident.ValueKind == JsonValueKind.Object)
            {
                if (ident.TryGetProperty("username_color", out var color)) msg.Color = color.GetString() ?? "";
                if (ident.TryGetProperty("badges", out var badges) && badges.ValueKind == JsonValueKind.Array) foreach (var b in badges.EnumerateArray())
                {
                    var type = NormalizeBadgeType(b.TryGetProperty("type", out var bt) ? bt.GetString() : ""); var text = b.TryGetProperty("text", out var tx) ? tx.GetString()?.Trim() ?? "" : ""; if (text.Length == 0) text = type; var count = b.TryGetProperty("count", out var co) && co.TryGetInt32(out var c) ? c : 0; if (text.Length > 0) msg.Badges.Add(text); if (type.Length > 0) { msg.BadgeDetails ??= []; msg.BadgeDetails.Add(new ChatBadge { Text = text, Type = type, Count = count }); types.Add(type); } if (type == "moderator") msg.IsMod = true;
                }
            }
            if (broadcaster != 0 && broadcaster == senderID && !types.Contains("broadcaster")) { msg.Badges.Insert(0, "Broadcaster"); msg.BadgeDetails ??= []; msg.BadgeDetails.Insert(0, new ChatBadge { Text = "Broadcaster", Type = "broadcaster" }); }
            return msg;
        }
        catch { return null; }
    }

    private void LoadSavedCredentials()
    {
        if (!settings.RememberKickLogin || !OperatingSystem.IsWindows() || !File.Exists(credentialsPath)) return;
        try
        {
            using var doc = JsonDocument.Parse(File.ReadAllBytes(credentialsPath)); var root = doc.RootElement; var id = root.GetProperty("client_id").GetString()?.Trim() ?? ""; var encoded = root.GetProperty("protected_secret").GetString()?.Trim() ?? ""; if (id.Length == 0 || encoded.Length == 0) return;
            var secret = Encoding.UTF8.GetString(AppUtil.UnprotectCredential(Convert.FromBase64String(encoded))).Trim(); if (secret.Length == 0) return;
            clientID = id; clientSecret = secret; authMode = "app"; updatedAt = AppUtil.NowMS();
        }
        catch { }
    }

    private async Task SaveCredentialsAsync(string id, string secret, bool remember)
    {
        if (!remember || id.Trim().Length == 0 || secret.Trim().Length == 0) { try { File.Delete(credentialsPath); } catch { } return; }
        if (!OperatingSystem.IsWindows()) return;
        var payload = new { version = 1, client_id = id.Trim(), protected_secret = Convert.ToBase64String(AppUtil.ProtectCredential(Encoding.UTF8.GetBytes(secret.Trim()))) };
        await AppUtil.AtomicWriteJsonAsync(credentialsPath, payload);
    }
    private Task SaveSettingsAsync() { ChatSettings copy; lock (gate) copy = Clone(settings); Normalize(copy); return AppUtil.AtomicWriteJsonAsync(settingsPath, copy); }
    private static T Clone<T>(T v) => JsonSerializer.Deserialize<T>(JsonSerializer.Serialize(v, AppUtil.Json), AppUtil.Json)!;
}
