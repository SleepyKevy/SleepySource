using System.Text.Json;

namespace SleepySource;

internal sealed class ChatService
{
    private readonly object gate = new();
    private readonly string settingsPath;
    private ChatSettings settings;
    private readonly List<ChatMessage> messages = [];
    private string connectedChannel = "";
    private string broadcasterUserID = "";
    private bool authReady;
    private bool liveChatConnected;
    private string liveChatStatus = "";
    private bool webhookSubscribed;
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
        settings = AppUtil.LoadJsonOrDefault(settingsPath, () => new ChatSettings(), "chat_settings");
        Normalize(settings);
        try { SaveSettingsAsync().GetAwaiter().GetResult(); } catch { }
    }

    public void ReloadFromDisk()
    {
        var next = AppUtil.LoadJsonOrDefault(settingsPath, () => new ChatSettings(), "chat settings");
        Normalize(next);
        lock (gate)
        {
            settings = next;
            messages.Clear();
            webhookSeen.Clear();
            webhookLastError = "";
            updatedAt = AppUtil.NowMS();
        }
    }

    public ChatState State()
    {
        lock (gate)
        {
            return new ChatState
            {
                Settings = Clone(settings),
                Messages = messages.Select(Clone).ToList(),
                UpdatedAt = updatedAt,
                AuthReady = authReady,
                AuthMode = authReady ? "hosted-oauth" : "",
                ConnectedChannel = connectedChannel,
                BroadcasterUserID = broadcasterUserID,
                LiveChatConnected = liveChatConnected,
                LiveChatStatus = liveChatStatus,
                WebhookSubscribed = webhookSubscribed,
                WebhookSubscriptionID = webhookSubscribed ? "managed" : "",
                WebhookLastEventAt = webhookLastEventAt,
                WebhookRequestCount = webhookRequestCount,
                WebhookVerifiedCount = webhookVerifiedCount,
                WebhookAcceptedCount = webhookAcceptedCount,
                WebhookRejectedCount = webhookRejectedCount,
                WebhookLastRequestAt = webhookLastRequestAt,
                WebhookLastEventType = webhookLastEventType,
                WebhookLastError = webhookLastError,
                CredentialStorage = OperatingSystem.IsWindows() ? "Windows encrypted SleepySource connection" : "local SleepySource connection"
            };
        }
    }

    public async Task SetSettingsAsync(ChatSettings next)
    {
        Normalize(next);
        lock (gate) { settings = next; updatedAt = AppUtil.NowMS(); }
        await SaveSettingsAsync();
    }

    public void SetHostedConnection(string channel, string userID)
    {
        channel = AppUtil.NormalizeKickChannelSlug(channel);
        lock (gate)
        {
            authReady = channel.Length > 0 && !string.IsNullOrWhiteSpace(userID);
            connectedChannel = channel;
            broadcasterUserID = (userID ?? "").Trim();
            if (channel.Length > 0) settings.KickChannel = channel;
            webhookLastError = "";
            liveChatStatus = authReady ? "Hosted Kick connection active" : "";
            updatedAt = AppUtil.NowMS();
        }
        _ = SaveSettingsAsync();
    }

    public void ClearHostedConnection()
    {
        lock (gate)
        {
            authReady = false;
            connectedChannel = "";
            broadcasterUserID = "";
            webhookSubscribed = false;
            liveChatConnected = false;
            liveChatStatus = "";
            webhookLastError = "";
            updatedAt = AppUtil.NowMS();
        }
    }

    public void SetHostedEventsState(bool subscribed, string status, string error = "")
    {
        lock (gate)
        {
            webhookSubscribed = subscribed;
            liveChatStatus = (status ?? "").Trim();
            webhookLastError = (error ?? "").Trim();
            updatedAt = AppUtil.NowMS();
        }
    }

    public void SetRealtimeState(bool connected, string status)
    {
        lock (gate)
        {
            liveChatConnected = connected;
            if (!string.IsNullOrWhiteSpace(status)) liveChatStatus = status.Trim();
            updatedAt = AppUtil.NowMS();
        }
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

    public void MarkHostedEvent(string type, bool accepted, string error = "")
    {
        lock (gate)
        {
            webhookRequestCount++;
            webhookLastRequestAt = AppUtil.NowMS();
            webhookLastEventType = (type ?? "").Trim();
            webhookVerifiedCount++;
            if (accepted)
            {
                webhookAcceptedCount++;
                webhookLastEventAt = webhookLastRequestAt;
                webhookLastError = "";
                if (webhookLastEventType.Equals("chat.message.sent", StringComparison.OrdinalIgnoreCase))
                {
                    liveChatConnected = true;
                    liveChatStatus = "Hosted Kick events active";
                }
            }
            else
            {
                webhookRejectedCount++;
                webhookLastError = (error ?? "Event could not be processed").Trim();
            }
            updatedAt = AppUtil.NowMS();
        }
    }

    public bool AcceptWebhookMessageID(string id)
    {
        id = id.Trim(); if (id.Length == 0) return true;
        lock (gate)
        {
            var now = AppUtil.NowMS();
            foreach (var k in webhookSeen.Where(x => x.Value < now - 86_400_000).Select(x => x.Key).ToList()) webhookSeen.Remove(k);
            if (webhookSeen.ContainsKey(id)) return false;
            webhookSeen[id] = now;
            return true;
        }
    }

    public static void Normalize(ChatSettings s)
    {
        s.SchemaVersion = Math.Max(7, s.SchemaVersion); s.CanvasWidth = AppUtil.Clamp(s.CanvasWidth, 320, 3840); s.CanvasHeight = AppUtil.Clamp(s.CanvasHeight, 240, 2160); s.BoxWidth = AppUtil.Clamp(s.BoxWidth, 180, 3840); s.BoxHeight = AppUtil.Clamp(s.BoxHeight, 120, 2160); s.BoxX = AppUtil.Clamp(s.BoxX, -3840, 3840); s.BoxY = AppUtil.Clamp(s.BoxY, -2160, 2160);
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

    private Task SaveSettingsAsync() { ChatSettings copy; lock (gate) copy = Clone(settings); Normalize(copy); return AppUtil.AtomicWriteJsonAsync(settingsPath, copy); }
    private static T Clone<T>(T v) => JsonSerializer.Deserialize<T>(JsonSerializer.Serialize(v, AppUtil.Json), AppUtil.Json)!;
}
