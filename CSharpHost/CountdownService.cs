namespace SleepySource;

internal sealed class CountdownService
{
    private readonly object gate = new();
    private readonly string settingsPath;
    private readonly string profileDir;
    private CountdownSettings settings;
    private long baseMS;
    private DateTime startAt = DateTime.UtcNow;
    private bool running;
    private bool paused;
    private bool finished;
    private bool hasStarted;
    private long updatedAtMS = AppUtil.NowMS();

    public CountdownService(string dataDir)
    {
        settingsPath = Path.Combine(dataDir, "countdown_settings.json");
        profileDir = Path.Combine(dataDir, "CountdownProfiles");
        Directory.CreateDirectory(profileDir);
        settings = AppUtil.LoadJsonOrDefault(settingsPath, () => new CountdownSettings(), "countdown_settings");
        Normalize(settings);
        baseMS = settings.Mode == "countdown" ? DurationMS(settings) : 0;
        if (settings.StartBehavior == "app-start") StartLocked(DateTime.UtcNow);
        try { SaveLockedAsync().GetAwaiter().GetResult(); } catch { }
    }

    public void ReloadFromDisk()
    {
        Directory.CreateDirectory(profileDir);
        var next = AppUtil.LoadJsonOrDefault(settingsPath, () => new CountdownSettings(), "countdown settings");
        Normalize(next);
        lock (gate) { settings = next; ResetLocked(DateTime.UtcNow, true); }
    }

    public CountdownState State(List<FontInfo>? fonts = null)
    {
        lock (gate)
        {
            var now = DateTime.UtcNow;
            var current = SettleLocked(now);
            return new CountdownState
            {
                Settings = Clone(settings), CurrentMS = current, DurationMS = DurationMS(settings), Running = running,
                Paused = paused, Finished = finished, HasStarted = hasStarted,
                DisplayText = DisplayText(settings, current, finished), ServerNowMS = new DateTimeOffset(now).ToUnixTimeMilliseconds(),
                UpdatedAtMS = updatedAtMS, OverlayURL = "http://127.0.0.1:17891/countdown",
                Fonts = fonts is { Count: > 0 } ? fonts : null, Profiles = ListProfiles() is { Count: > 0 } profiles ? profiles : null
            };
        }
    }

    public async Task ApplySettingsAsync(CountdownSettings next)
    {
        Normalize(next);
        lock (gate)
        {
            var now = DateTime.UtcNow;
            var current = SettleLocked(now);
            var oldMode = settings.Mode;
            var oldDuration = DurationMS(settings);
            var wasRunning = running; var wasPaused = paused; var hadStarted = hasStarted;
            settings = next;
            var newDuration = DurationMS(next);
            if (oldMode != next.Mode) ResetLocked(now, true);
            else if (next.Mode == "countdown" && !wasRunning && oldDuration != newDuration) { baseMS = newDuration; finished = false; }
            else { baseMS = current; startAt = now; running = wasRunning; paused = wasPaused; hasStarted = hadStarted; }
            updatedAtMS = AppUtil.NowMS();
        }
        await SaveLockedAsync();
    }

    public void Control(string action)
    {
        lock (gate)
        {
            var now = DateTime.UtcNow;
            switch (action)
            {
                case "start": StartLocked(now); break;
                case "pause": PauseLocked(now); break;
                case "stop": StopLocked(now); break;
                case "toggle": if (running) PauseLocked(now); else StartLocked(now); break;
                case "reset": ResetLocked(now, false); break;
                case "add60": AdjustLocked(now, 60000); break;
                case "sub60": AdjustLocked(now, -60000); break;
                case "add10": AdjustLocked(now, 10000); break;
                case "sub10": AdjustLocked(now, -10000); break;
                case "overlay_loaded":
                    if (settings.StartBehavior == "overlay-load")
                    {
                        if (settings.RestartOnLoad) { ResetLocked(now, false); StartLocked(now); }
                        else if (!hasStarted && !running) StartLocked(now);
                    }
                    break;
                case "overlay_unloaded": if (settings.ResetOnUnload) ResetLocked(now, false); break;
                default: throw new InvalidOperationException($"unknown countdown action {action}");
            }
        }
    }

    public List<ProfileInfo> ListProfiles() => Directory.EnumerateFiles(profileDir, "*.json")
        .Select(p => new ProfileInfo { Name = Path.GetFileNameWithoutExtension(p), ModifiedAt = new DateTimeOffset(File.GetLastWriteTimeUtc(p)).ToUnixTimeMilliseconds() })
        .OrderBy(p => p.Name, StringComparer.OrdinalIgnoreCase).ToList();

    public async Task SaveProfileAsync(string name)
    {
        name = AppUtil.SanitizeProfileName(name); if (name.Length == 0) throw new InvalidDataException("enter a profile name");
        CountdownSettings copy; lock (gate) copy = Clone(settings); Normalize(copy);
        await AppUtil.AtomicWriteJsonAsync(Path.Combine(profileDir, name + ".json"), copy);
    }

    public async Task LoadProfileAsync(string name)
    {
        name = AppUtil.SanitizeProfileName(name); if (name.Length == 0) throw new InvalidDataException("choose a profile");
        var path = Path.Combine(profileDir, name + ".json"); if (!File.Exists(path)) throw new FileNotFoundException("profile not found");
        var next = System.Text.Json.JsonSerializer.Deserialize<CountdownSettings>(await File.ReadAllBytesAsync(path), AppUtil.Json) ?? throw new InvalidDataException("profile is not valid JSON");
        await ApplySettingsAsync(next); Control("reset");
    }

    public void DeleteProfile(string name)
    {
        name = AppUtil.SanitizeProfileName(name); if (name.Length == 0) throw new InvalidDataException("choose a profile");
        try { File.Delete(Path.Combine(profileDir, name + ".json")); } catch { throw; }
    }

    public static void Normalize(CountdownSettings s)
    {
        s.SchemaVersion = 2; if (s.Mode is not ("countdown" or "stopwatch")) s.Mode = "countdown";
        s.Hours = AppUtil.Clamp(s.Hours, 0, 999); s.Minutes = AppUtil.Clamp(s.Minutes, 0, 59); s.Seconds = AppUtil.Clamp(s.Seconds, 0, 59);
        if (!new[] { "auto", "hhmmss", "mmss", "seconds", "custom" }.Contains(s.Format)) s.Format = "auto";
        s.CustomFormat = (s.CustomFormat ?? ""); if (s.CustomFormat.Length > 256) s.CustomFormat = s.CustomFormat[..256]; if (string.IsNullOrWhiteSpace(s.CustomFormat)) s.CustomFormat = "{hh}:{mm}:{ss}";
        if (s.Prefix.Length > 128) s.Prefix = s.Prefix[..128]; if (s.Suffix.Length > 128) s.Suffix = s.Suffix[..128]; if (s.FinishedText.Length > 256) s.FinishedText = s.FinishedText[..256];
        if (!new[] { "manual", "app-start", "overlay-load" }.Contains(s.StartBehavior)) s.StartBehavior = "manual";
        s.CanvasWidth = AppUtil.Clamp(s.CanvasWidth, 100, 3840); s.CanvasHeight = AppUtil.Clamp(s.CanvasHeight, 60, 2160); s.CanvasColor = AppUtil.NormalizeColor(s.CanvasColor, "#000000"); s.CanvasOpacity = AppUtil.Clamp(s.CanvasOpacity, 0, 100);
        s.TimerWidth = AppUtil.Clamp(s.TimerWidth, 40, 3840); s.TimerX = AppUtil.Clamp(s.TimerX, -3840, 3840); s.TimerY = AppUtil.Clamp(s.TimerY, -2160, 2160);
        if (string.IsNullOrWhiteSpace(s.FontFamily) || s.FontFamily.Length > 128) s.FontFamily = "Segoe UI"; s.FontSize = AppUtil.Clamp(s.FontSize, 8, 400); if (!new[] { 100,200,300,400,500,600,700,800,900 }.Contains(s.FontWeight)) s.FontWeight = 700;
        s.TextColor = AppUtil.NormalizeColor(s.TextColor, "#FFFFFF"); s.TextOpacity = AppUtil.Clamp(s.TextOpacity, 0, 100); if (!new[] { "left", "center", "right" }.Contains(s.Align)) s.Align = "center";
        s.LetterSpacing = AppUtil.Clamp(s.LetterSpacing, -10, 50); s.LineHeight = AppUtil.Clamp(s.LineHeight, .6, 3); s.OutlineSize = AppUtil.Clamp(s.OutlineSize, 0, 20); s.OutlineColor = AppUtil.NormalizeColor(s.OutlineColor, "#000000"); s.OutlineOpacity = AppUtil.Clamp(s.OutlineOpacity, 0, 100);
        s.PanelColor = AppUtil.NormalizeColor(s.PanelColor, "#07111F"); s.PanelOpacity = AppUtil.Clamp(s.PanelOpacity, 0, 100); s.PanelRadius = AppUtil.Clamp(s.PanelRadius, 0, 200); s.PanelPadding = AppUtil.Clamp(s.PanelPadding, 0, 120); s.BorderWidth = AppUtil.Clamp(s.BorderWidth, 0, 20); s.BorderColor = AppUtil.NormalizeColor(s.BorderColor, "#3AA7FF"); s.BorderOpacity = AppUtil.Clamp(s.BorderOpacity, 0, 100);
        s.AnimationMS = AppUtil.Clamp(s.AnimationMS, 250, 12000);
        if (!new[] { "float", "pulse", "breathe", "glow", "tilt", "none" }.Contains(s.TimerAnimation)) s.TimerAnimation = "none";
        if (!new[] { "pop", "flip", "slide", "pulse", "none" }.Contains(s.TickAnimation)) s.TickAnimation = "none";
        if (!new[] { "breathe", "glow", "shimmer", "pulse", "none" }.Contains(s.PanelAnimation)) s.PanelAnimation = "none";
        if (!new[] { "fade", "slide-up", "zoom", "none" }.Contains(s.OverlayAnimation)) s.OverlayAnimation = "none";
    }

    public static long DurationMS(CountdownSettings s) => ((long)s.Hours * 3600 + (long)s.Minutes * 60 + s.Seconds) * 1000;
    public static string DisplayText(CountdownSettings s, long ms, bool finished)
    {
        string body;
        if (finished) { if (s.BlankOnFinish) return ""; body = s.FinishedText.Length > 0 ? s.FinishedText : FormatBody(s, 0); }
        else body = FormatBody(s, DisplaySeconds(s.Mode, ms));
        return s.Prefix + body + s.Suffix;
    }

    private static long DisplaySeconds(string mode, long ms) => mode == "countdown" ? (ms > 0 ? (ms + 999) / 1000 : ms < 0 ? -((-ms + 999) / 1000) : 0) : ms / 1000;
    private static string FormatBody(CountdownSettings s, long seconds)
    {
        var neg = seconds < 0; var abs = (ulong)(neg ? -(seconds + 1) + 1 : seconds); var sign = neg ? "-" : "";
        var days = abs / 86400; var totalHours = abs / 3600; var hc = totalHours % 24; var totalMinutes = abs / 60; var minutes = totalMinutes % 60; var secs = abs % 60;
        return s.Format switch
        {
            "hhmmss" => $"{sign}{totalHours:00}:{minutes:00}:{secs:00}",
            "mmss" => $"{sign}{totalMinutes:00}:{secs:00}",
            "seconds" => $"{sign}{abs}",
            "custom" => FormatCustom(s.CustomFormat, seconds),
            _ when abs >= 86400 => $"{sign}{days}:{hc:00}:{minutes:00}:{secs:00}",
            _ when abs >= 3600 => $"{sign}{totalHours}:{minutes:00}:{secs:00}",
            _ => $"{sign}{totalMinutes:00}:{secs:00}"
        };
    }

    private static string FormatCustom(string format, long seconds)
    {
        var neg = seconds < 0; var abs = (ulong)(neg ? -(seconds + 1) + 1 : seconds);
        var days = abs / 86400; var hours = (abs / 3600) % 24; var minutes = (abs / 60) % 60; var secs = abs % 60;
        return format.Replace("{sign}", neg ? "-" : "").Replace("{hh}", hours.ToString("00")).Replace("{mm}", minutes.ToString("00")).Replace("{ss}", secs.ToString("00"))
            .Replace("{d}", days.ToString()).Replace("{h}", hours.ToString()).Replace("{m}", minutes.ToString()).Replace("{s}", secs.ToString());
    }

    private long CurrentLocked(DateTime now)
    {
        if (!running) return baseMS; var elapsed = Math.Max(0, (long)(now - startAt).TotalMilliseconds); return settings.Mode == "countdown" ? baseMS - elapsed : baseMS + elapsed;
    }
    private long SettleLocked(DateTime now)
    {
        var value = CurrentLocked(now); if (settings.Mode != "countdown" || !running || settings.Overtime || value > 0) return value;
        var duration = DurationMS(settings); if (settings.Loop && duration > 0) { var cycles = ((-value) / duration) + 1; value += cycles * duration; baseMS = value; startAt = now; finished = false; return value; }
        baseMS = 0; running = false; paused = false; finished = true; updatedAtMS = AppUtil.NowMS(); return 0;
    }
    private void ResetLocked(DateTime now, bool clearStarted) { running = false; paused = false; finished = false; if (clearStarted) hasStarted = false; baseMS = settings.Mode == "countdown" ? DurationMS(settings) : 0; startAt = now; updatedAtMS = AppUtil.NowMS(); }
    private void StartLocked(DateTime now) { if (running) return; if (settings.Mode == "countdown" && finished) { baseMS = DurationMS(settings); finished = false; } startAt = now; running = true; paused = false; hasStarted = true; updatedAtMS = AppUtil.NowMS(); }
    private void PauseLocked(DateTime now) { if (!running) return; baseMS = SettleLocked(now); if (finished) return; running = false; paused = true; updatedAtMS = AppUtil.NowMS(); }
    private void StopLocked(DateTime now) { if (!running) { if (!paused && !hasStarted) return; paused = false; updatedAtMS = AppUtil.NowMS(); return; } baseMS = SettleLocked(now); if (finished) return; running = false; paused = false; hasStarted = true; startAt = now; updatedAtMS = AppUtil.NowMS(); }
    private void AdjustLocked(DateTime now, long delta) { var value = SettleLocked(now) + delta; if (!settings.Overtime && value < 0) value = 0; baseMS = value; startAt = now; finished = false; updatedAtMS = AppUtil.NowMS(); }
    private Task SaveLockedAsync() { CountdownSettings copy; lock (gate) copy = Clone(settings); return AppUtil.AtomicWriteJsonAsync(settingsPath, copy); }
    private static T Clone<T>(T value) => System.Text.Json.JsonSerializer.Deserialize<T>(System.Text.Json.JsonSerializer.Serialize(value, AppUtil.Json), AppUtil.Json)!;
}
