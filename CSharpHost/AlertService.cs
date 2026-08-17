using System.Text.Json;

namespace SleepySource;

internal sealed class AlertService
{
    public static readonly string[] AlertTypes = ["follow", "subscription-new", "subscription-renewal", "subscription-gift", "kicks", "reward"];
    private readonly object gate = new();
    private readonly string settingsPath;
    public string MediaDir { get; }
    private AlertSettings settings;
    private readonly List<AlertEvent> queue = [];
    private AlertPresentation? current;
    private readonly Dictionary<string, long> seen = new(StringComparer.Ordinal);
    private long dropped;
    private long updatedAtMS = AppUtil.NowMS();
    private long sequence;
    private AlertEvent? lastEvent;

    public AlertService(string dataDir)
    {
        settingsPath = Path.Combine(dataDir, "alert_settings.json");
        MediaDir = Path.Combine(dataDir, "Alerts");
        Directory.CreateDirectory(MediaDir);
        settings = AppUtil.LoadJsonOrDefault(settingsPath, DefaultSettings, "alert_settings");
        settings = NormalizeSettings(settings);
        try { SaveAsync().GetAwaiter().GetResult(); } catch { }
    }

    public AlertSettings SettingsSnapshot() { lock (gate) return Clone(settings); }
    public async Task SetSettingsAsync(AlertSettings next) { next = NormalizeSettings(next); lock (gate) { settings = next; updatedAtMS = AppUtil.NowMS(); } await SaveAsync(); }

    public bool Enqueue(AlertEvent ev, string dedupeKey = "")
    {
        if (!AlertTypes.Contains(ev.Type)) return false;
        ev.Source = string.IsNullOrWhiteSpace(ev.Source) ? "local" : ev.Source.Trim();
        ev.Username = string.IsNullOrWhiteSpace(ev.Username) ? "Anonymous" : ev.Username.Trim();
        if (ev.CreatedAtMS <= 0) ev.CreatedAtMS = AppUtil.NowMS();
        lock (gate)
        {
            var now = AppUtil.NowMS(); CleanupSeen(now);
            dedupeKey = dedupeKey.Trim();
            if (dedupeKey.Length > 0)
            {
                if (seen.ContainsKey(dedupeKey)) return false;
                seen[dedupeKey] = now;
            }
            var style = settings.Types[ev.Type];
            if (!style.Enabled) return false;
            if (string.IsNullOrWhiteSpace(ev.ID)) ev.ID = $"alert-{now}-{++sequence}";
            if (queue.Count >= settings.QueueLimit) { dropped++; updatedAtMS = now; return false; }
            queue.Add(Clone(ev)); lastEvent = Clone(ev); updatedAtMS = now; return true;
        }
    }

    public AlertState State(bool consume)
    {
        lock (gate)
        {
            var now = AppUtil.NowMS(); if (consume) Advance(now);
            return new AlertState
            {
                Settings = Clone(settings), Current = current == null ? null : Clone(current), Queue = queue.Select(Clone).ToList(), QueueDepth = queue.Count,
                Dropped = dropped, LastEvent = lastEvent == null ? null : Clone(lastEvent), UpdatedAtMS = updatedAtMS,
                OverlayURL = "http://127.0.0.1:17891/alerts"
            };
        }
    }

    public void Control(string action)
    {
        lock (gate)
        {
            var now = AppUtil.NowMS();
            switch (action.Trim().ToLowerInvariant())
            {
                case "skip": current = null; updatedAtMS = now; break;
                case "clear": current = null; queue.Clear(); updatedAtMS = now; break;
                default: throw new InvalidDataException("unknown alert control action");
            }
        }
    }

    public AlertEvent Sample(string type)
    {
        var now = AppUtil.NowMS();
        return type switch
        {
            "follow" => new AlertEvent { Type = type, Source = "test", Username = "SleepyViewer", CreatedAtMS = now },
            "subscription-new" => new AlertEvent { Type = type, Source = "test", Username = "SleepySubscriber", Tier = "Tier 1", CreatedAtMS = now },
            "subscription-renewal" => new AlertEvent { Type = type, Source = "test", Username = "SleepyRegular", Months = 6, Tier = "Tier 1", CreatedAtMS = now },
            "subscription-gift" => new AlertEvent { Type = type, Source = "test", Username = "SleepyGifter", Count = 5, Tier = "Tier 1", CreatedAtMS = now },
            "kicks" => new AlertEvent { Type = type, Source = "test", Username = "SleepySupporter", Amount = 500, GiftName = "500 Kicks", CreatedAtMS = now },
            "reward" => new AlertEvent { Type = type, Source = "test", Username = "SleepyViewer", RewardTitle = "Hydrate", UserInput = "Water break!", CreatedAtMS = now },
            _ => throw new InvalidDataException("unknown alert type")
        };
    }

    public string? MediaPath(string type, string kind)
    {
        if (!AlertTypes.Contains(type)) return null;
        var style = SettingsSnapshot().Types[type];
        var name = kind == "sound" ? style.SoundFile : style.VisualFile;
        if (string.IsNullOrWhiteSpace(name)) return null;
        var path = Path.Combine(MediaDir, type, Path.GetFileName(name));
        return File.Exists(path) ? path : null;
    }

    public async Task SetMediaAsync(string type, string kind, string fileName)
    {
        var next = SettingsSnapshot(); if (!next.Types.TryGetValue(type, out var style)) throw new InvalidDataException("unknown alert type");
        var now = AppUtil.NowMS();
        if (kind == "sound") { style.SoundFile = Path.GetFileName(fileName); style.SoundUpdatedAt = now; }
        else { style.VisualFile = Path.GetFileName(fileName); style.VisualUpdatedAt = now; }
        next.Types[type] = style; await SetSettingsAsync(next);
    }

    public async Task RemoveMediaAsync(string type, string kind)
    {
        var next = SettingsSnapshot(); if (!next.Types.TryGetValue(type, out var style)) throw new InvalidDataException("unknown alert type");
        var name = kind == "sound" ? style.SoundFile : style.VisualFile;
        if (!string.IsNullOrWhiteSpace(name)) try { File.Delete(Path.Combine(MediaDir, type, Path.GetFileName(name))); } catch { }
        if (kind == "sound") { style.SoundFile = ""; style.SoundUpdatedAt = 0; } else { style.VisualFile = ""; style.VisualUpdatedAt = 0; }
        next.Types[type] = style; await SetSettingsAsync(next);
    }

    public static AlertSettings DefaultSettings()
    {
        var s = new AlertSettings();
        foreach (var type in AlertTypes) s.Types[type] = DefaultStyle(type);
        return s;
    }

    public static AlertStyle DefaultStyle(string type)
    {
        var s = new AlertStyle();
        switch (type)
        {
            case "follow": s.TitleTemplate = "{user} followed!"; s.MessageTemplate = "Welcome to the stream."; break;
            case "subscription-new": s.TitleTemplate = "Thanks for the sub, {user}!"; s.MessageTemplate = "Welcome to the community!"; s.AccentColor = "#7C8CFF"; break;
            case "subscription-renewal": s.TitleTemplate = "{user} resubscribed!"; s.MessageTemplate = "{months} month subscription"; s.AccentColor = "#8D79FF"; break;
            case "subscription-gift": s.TitleTemplate = "Gift subs!"; s.MessageTemplate = "{user} gifted {count} subscription{plural}!"; s.AccentColor = "#C77DFF"; break;
            case "kicks": s.TitleTemplate = "{gift}"; s.MessageTemplate = "{user} sent {amount} Kicks!"; s.AccentColor = "#53E58C"; break;
            case "reward": s.TitleTemplate = "{reward}"; s.MessageTemplate = "Redeemed by {user}{input_suffix}"; s.AccentColor = "#FFB85C"; break;
        }
        return s;
    }

    public static AlertSettings NormalizeSettings(AlertSettings? input)
    {
        var s = input ?? DefaultSettings(); var old = s.SchemaVersion; s.SchemaVersion = 2;
        s.CanvasWidth = AppUtil.Clamp(s.CanvasWidth, 640, 3840); s.CanvasHeight = AppUtil.Clamp(s.CanvasHeight, 360, 2160); s.QueueLimit = AppUtil.Clamp(s.QueueLimit, 1, 100);
        s.Types ??= new(StringComparer.Ordinal);
        foreach (var type in AlertTypes)
        {
            var fallback = DefaultStyle(type); if (!s.Types.TryGetValue(type, out var style)) style = fallback;
            if (old < 2) MigrateV1(style, fallback);
            s.Types[type] = NormalizeStyle(style, fallback, s.CanvasWidth, s.CanvasHeight);
        }
        foreach (var key in s.Types.Keys.Where(k => !AlertTypes.Contains(k)).ToList()) s.Types.Remove(key);
        return s;
    }

    private static void MigrateV1(AlertStyle s, AlertStyle f)
    {
        s.DisplayMode = "card"; s.ShowTitle = s.ShowMessage = true; s.EnterAnimation = string.IsNullOrWhiteSpace(s.Animation) ? f.EnterAnimation : s.Animation; s.ExitAnimation = f.ExitAnimation;
        s.EnterDurationMS = f.EnterDurationMS; s.ExitDurationMS = f.ExitDurationMS; s.SnapEnabled = true; s.MediaFit = "contain"; s.MediaOpacity = 100; s.MediaX = 34; s.MediaY = Math.Max(18, (s.Height - s.MediaHeight) / 2);
        s.TitleX = 235; s.TitleY = 105; s.TitleWidth = Math.Max(160, s.Width - 275); s.TitleHeight = f.TitleHeight; s.TitleFontFamily = f.TitleFontFamily; s.TitleWeight = f.TitleWeight; s.TitleColor = AppUtil.NormalizeColor(s.TextColor, f.TitleColor); s.TitleAlign = f.TitleAlign; s.TitleOutlineColor = f.TitleOutlineColor; s.TitleLineHeight = f.TitleLineHeight; s.TitleShadow = s.Shadow;
        s.MessageX = 235; s.MessageY = 175; s.MessageWidth = Math.Max(160, s.Width - 275); s.MessageHeight = f.MessageHeight; s.MessageFontFamily = f.MessageFontFamily; s.MessageWeight = f.MessageWeight; s.MessageColor = AppUtil.NormalizeColor(s.TextColor, f.MessageColor); s.MessageAlign = f.MessageAlign; s.MessageOutlineColor = f.MessageOutlineColor; s.MessageLineHeight = f.MessageLineHeight; s.MessageShadow = s.Shadow;
    }

    private static AlertStyle NormalizeStyle(AlertStyle s, AlertStyle f, int cw, int ch)
    {
        s.TitleTemplate = string.IsNullOrWhiteSpace(s.TitleTemplate) ? f.TitleTemplate : s.TitleTemplate; if (s.TitleTemplate.Length > 256) s.TitleTemplate = s.TitleTemplate[..256];
        s.MessageTemplate = string.IsNullOrWhiteSpace(s.MessageTemplate) ? f.MessageTemplate : s.MessageTemplate; if (s.MessageTemplate.Length > 512) s.MessageTemplate = s.MessageTemplate[..512];
        if (!new[] { "card", "custom", "media-only", "text-only" }.Contains(s.DisplayMode)) s.DisplayMode = f.DisplayMode;
        s.DurationMS = AppUtil.Clamp(s.DurationMS, 500, 30000); s.EnterAnimation = NormalizeAnim(s.EnterAnimation, f.EnterAnimation, true); s.ExitAnimation = NormalizeAnim(s.ExitAnimation, f.ExitAnimation, false); s.EnterDurationMS = AppUtil.Clamp(s.EnterDurationMS, 0, 5000); s.ExitDurationMS = AppUtil.Clamp(s.ExitDurationMS, 0, 5000);
        s.Width = AppUtil.Clamp(s.Width, 120, cw); s.Height = AppUtil.Clamp(s.Height, 80, ch); s.X = AppUtil.Clamp(s.X, -cw, cw); s.Y = AppUtil.Clamp(s.Y, -ch, ch);
        s.MediaX = AppUtil.Clamp(s.MediaX, -s.Width, s.Width); s.MediaY = AppUtil.Clamp(s.MediaY, -s.Height, s.Height); s.MediaWidth = AppUtil.Clamp(s.MediaWidth, 20, cw); s.MediaHeight = AppUtil.Clamp(s.MediaHeight, 20, ch); if (!new[] { "contain", "cover", "fill", "none" }.Contains(s.MediaFit)) s.MediaFit = f.MediaFit; s.MediaOpacity = AppUtil.Clamp(s.MediaOpacity, 0, 100); s.MediaRotation = AppUtil.Clamp(s.MediaRotation, -180, 180);
        s.TitleX = AppUtil.Clamp(s.TitleX, -s.Width, s.Width); s.TitleY = AppUtil.Clamp(s.TitleY, -s.Height, s.Height); s.TitleWidth = AppUtil.Clamp(s.TitleWidth, 40, cw); s.TitleHeight = AppUtil.Clamp(s.TitleHeight, 20, ch); s.TitleFontFamily = NormalizeFont(s.TitleFontFamily, f.TitleFontFamily); s.TitleSize = AppUtil.Clamp(s.TitleSize, 8, 200); s.TitleWeight = AppUtil.Clamp(s.TitleWeight, 100, 900); s.TitleColor = AppUtil.NormalizeColor(s.TitleColor, f.TitleColor); s.TitleAlign = NormalizeAlign(s.TitleAlign, f.TitleAlign); s.TitleOutlineColor = AppUtil.NormalizeColor(s.TitleOutlineColor, f.TitleOutlineColor); s.TitleOutlineWidth = AppUtil.Clamp(s.TitleOutlineWidth, 0, 12); s.TitleLetterSpacing = AppUtil.Clamp(s.TitleLetterSpacing, -10, 30); s.TitleLineHeight = AppUtil.Clamp(s.TitleLineHeight, 70, 220);
        s.MessageX = AppUtil.Clamp(s.MessageX, -s.Width, s.Width); s.MessageY = AppUtil.Clamp(s.MessageY, -s.Height, s.Height); s.MessageWidth = AppUtil.Clamp(s.MessageWidth, 40, cw); s.MessageHeight = AppUtil.Clamp(s.MessageHeight, 20, ch); s.MessageFontFamily = NormalizeFont(s.MessageFontFamily, f.MessageFontFamily); s.MessageSize = AppUtil.Clamp(s.MessageSize, 8, 160); s.MessageWeight = AppUtil.Clamp(s.MessageWeight, 100, 900); s.MessageColor = AppUtil.NormalizeColor(s.MessageColor, f.MessageColor); s.MessageAlign = NormalizeAlign(s.MessageAlign, f.MessageAlign); s.MessageOutlineColor = AppUtil.NormalizeColor(s.MessageOutlineColor, f.MessageOutlineColor); s.MessageOutlineWidth = AppUtil.Clamp(s.MessageOutlineWidth, 0, 12); s.MessageLetterSpacing = AppUtil.Clamp(s.MessageLetterSpacing, -10, 30); s.MessageLineHeight = AppUtil.Clamp(s.MessageLineHeight, 70, 220);
        s.BackgroundColor = AppUtil.NormalizeColor(s.BackgroundColor, f.BackgroundColor); s.BackgroundOpacity = AppUtil.Clamp(s.BackgroundOpacity, 0, 100); s.AccentColor = AppUtil.NormalizeColor(s.AccentColor, f.AccentColor); s.Radius = AppUtil.Clamp(s.Radius, 0, 120); s.BorderWidth = AppUtil.Clamp(s.BorderWidth, 0, 20); s.SoundVolume = AppUtil.Clamp(s.SoundVolume, 0, 100); s.SoundDelayMS = AppUtil.Clamp(s.SoundDelayMS, 0, 10000); s.VisualFile = Path.GetFileName((s.VisualFile ?? "").Trim()); s.SoundFile = Path.GetFileName((s.SoundFile ?? "").Trim()); s.Animation = s.EnterAnimation; s.TextColor = s.TitleColor;
        return s;
    }

    private static string NormalizeFont(string? v, string fallback) => new[] { "Segoe UI", "Arial", "Verdana", "Tahoma", "Trebuchet MS", "Georgia", "Times New Roman", "Impact" }.Contains((v ?? "").Trim()) ? v!.Trim() : fallback;
    private static string NormalizeAlign(string? v, string fallback) => new[] { "left", "center", "right" }.Contains((v ?? "").Trim()) ? v!.Trim() : fallback;
    private static string NormalizeAnim(string? v, string fallback, bool pop) { var x = (v ?? "").Trim(); if (new[] { "none", "fade", "slide-up", "slide-down", "slide-left", "slide-right", "zoom" }.Contains(x) || (pop && x == "pop")) return x; return fallback; }

    private void Advance(long now) { if (current != null && now >= current.EndsAtMS) { current = null; updatedAtMS = now; } if (current == null) StartNext(now); }
    private void StartNext(long now)
    {
        while (queue.Count > 0)
        {
            var ev = queue[0]; queue.RemoveAt(0); if (!settings.Types.TryGetValue(ev.Type, out var st) || !st.Enabled) continue;
            current = new AlertPresentation
            {
                ID = ev.ID, Type = ev.Type, Source = ev.Source, Username = ev.Username, Amount = ev.Amount, Count = ev.Count, Months = ev.Months, Tier = ev.Tier, GiftName = ev.GiftName, RewardTitle = ev.RewardTitle, UserInput = ev.UserInput, CreatedAtMS = ev.CreatedAtMS,
                Style = Clone(st), Title = RenderTemplate(st.TitleTemplate, ev), Message = RenderTemplate(st.MessageTemplate, ev), StartedAtMS = now, EndsAtMS = now + st.EnterDurationMS + st.DurationMS + st.ExitDurationMS,
                VisualURL = st.VisualFile.Length > 0 ? $"/media/alerts?type={Uri.EscapeDataString(ev.Type)}&kind=visual&v={st.VisualUpdatedAt}" : "",
                SoundURL = st.SoundFile.Length > 0 ? $"/media/alerts?type={Uri.EscapeDataString(ev.Type)}&kind=sound&v={st.SoundUpdatedAt}" : ""
            }; updatedAtMS = now; return;
        }
    }
    private void CleanupSeen(long now) { var cutoff = now - (long)TimeSpan.FromHours(24).TotalMilliseconds; foreach (var key in seen.Where(x => x.Value < cutoff).Select(x => x.Key).ToList()) seen.Remove(key); if (seen.Count > 8192) foreach (var key in seen.Keys.Take(seen.Count - 4096).ToList()) seen.Remove(key); }
    public static string RenderTemplate(string template, AlertEvent ev)
    {
        var plural = ev.Count == 1 ? "" : "s"; var suffix = string.IsNullOrWhiteSpace(ev.UserInput) ? "" : ": " + ev.UserInput.Trim();
        return (template ?? "").Replace("{user}", ev.Username).Replace("{count}", ev.Count.ToString()).Replace("{plural}", plural).Replace("{amount}", ev.Amount.ToString()).Replace("{months}", ev.Months.ToString()).Replace("{tier}", ev.Tier).Replace("{gift}", ev.GiftName).Replace("{reward}", ev.RewardTitle).Replace("{input}", ev.UserInput).Replace("{input_suffix}", suffix).Trim();
    }
    private Task SaveAsync() { AlertSettings copy; lock (gate) copy = Clone(settings); return AppUtil.AtomicWriteJsonAsync(settingsPath, copy); }
    private static T Clone<T>(T v) => JsonSerializer.Deserialize<T>(JsonSerializer.Serialize(v, AppUtil.Json), AppUtil.Json)!;
}
