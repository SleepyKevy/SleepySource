using System.Text.Json;

namespace SleepySource;

internal sealed class CoreStateService
{
    private readonly object gate = new();
    private readonly object outputGate = new();
    private string lastOutput = "\0";

    public string ExeDir { get; } = AppContext.BaseDirectory;
    public string DataDir { get; }
    public string MediaDir { get; }
    public string FontDir { get; }
    public string ProfileDir { get; }
    public string SettingsPath { get; }
    public string OutputPath { get; }

    private Settings settings = new();
    private Track track = new();
    private string detector = "Starting Windows media-session detector…";
    private long updatedAt = AppUtil.NowMS();
    private double overlayFPS;
    private double overlayFrameMS;
    private long overlayMetricsAt;

    public CoreStateService()
    {
        DataDir = Path.Combine(ExeDir, "SleepySource_Data");
        MediaDir = Path.Combine(DataDir, "Media");
        FontDir = Path.Combine(MediaDir, "fonts");
        ProfileDir = Path.Combine(DataDir, "Profiles");
        SettingsPath = Path.Combine(DataDir, "settings.json");
        OutputPath = Path.Combine(DataDir, "now_playing.txt");
        foreach (var dir in new[] { DataDir, MediaDir, FontDir, ProfileDir }) Directory.CreateDirectory(dir);
        MigrateLegacyPortableData();
        settings = AppUtil.LoadJsonOrDefault(SettingsPath, () => new Settings(), "settings");
        NormalizeSettings(settings);
        FindExistingMedia();
        try { AppUtil.AtomicWriteJsonAsync(SettingsPath, settings).GetAwaiter().GetResult(); } catch { }
    }

    public void ReloadFromDisk()
    {
        foreach (var dir in new[] { DataDir, MediaDir, FontDir, ProfileDir }) Directory.CreateDirectory(dir);
        var next = AppUtil.LoadJsonOrDefault(SettingsPath, () => new Settings(), "settings");
        NormalizeSettings(next);
        lock (gate) { settings = next; updatedAt = AppUtil.NowMS(); }
        FindExistingMedia();
        WriteLegacyOutput();
    }

    public Settings SettingsSnapshot()
    {
        lock (gate) return Clone(settings);
    }

    public async Task<Settings> ApplySettingsAsync(Settings next, CancellationToken ct = default)
    {
        NormalizeSettings(next);
        lock (gate)
        {
            settings = next;
            updatedAt = AppUtil.NowMS();
        }
        await AppUtil.AtomicWriteJsonAsync(SettingsPath, next, ct);
        WriteLegacyOutput();
        return SettingsSnapshot();
    }

    public void SetOverlayMetrics(double fps, double frameMS)
    {
        lock (gate)
        {
            overlayFPS = Math.Round(AppUtil.Clamp(fps, 0, 1000), 2);
            overlayFrameMS = Math.Round(AppUtil.Clamp(frameMS, 0, 10000), 2);
            overlayMetricsAt = AppUtil.NowMS();
        }
    }

    public void UpdateTrack(Track next, string detectorStatus)
    {
        next.SampledAtMS = AppUtil.NowMS();
        next.PositionMS = Math.Max(0, next.PositionMS);
        next.DurationMS = Math.Max(0, next.DurationMS);
        if (next.DurationMS > 0) next.PositionMS = Math.Min(next.DurationMS, next.PositionMS);
        lock (gate)
        {
            if (!TrackEquals(track, next)) updatedAt = AppUtil.NowMS();
            track = next;
            if (!string.IsNullOrWhiteSpace(detectorStatus)) detector = detectorStatus;
        }
        WriteLegacyOutput();
    }

    public void SetDetectorStatus(string status)
    {
        lock (gate) detector = status;
    }

    public AppState Snapshot()
    {
        Settings s;
        Track t;
        string d;
        long u;
        double fps, frame;
        long metricsAt;
        lock (gate)
        {
            s = Clone(settings);
            t = Clone(track);
            d = detector;
            u = updatedAt;
            fps = overlayFPS;
            frame = overlayFrameMS;
            metricsAt = overlayMetricsAt;
        }

        var display = RenderText(t, s);
        var visible = (!t.Found ? s.ShowWhenIdle : !(s.HideWhenPaused && !t.Status.Equals("Playing", StringComparison.OrdinalIgnoreCase)));
        var custom = ResolveMediaFile(s.CustomImage, "custom_now_playing");
        var background = ResolveMediaFile(s.CustomBackground, "custom_background");

        var imageUrl = "/assets/default.png";
        var imageName = "Built-in fallback artwork";
        var mediaKind = "image";
        if (s.ImageMode == "custom")
        {
            if (custom != null)
            {
                imageUrl = "/media/custom?v=" + File.GetLastWriteTimeUtc(custom).Ticks;
                imageName = Path.GetFileName(custom);
                mediaKind = Path.GetExtension(custom).Equals(".webm", StringComparison.OrdinalIgnoreCase) ? "video" : "image";
            }
            else imageName = "Custom image not set — using fallback";
        }

        var backgroundUrl = "";
        var backgroundName = "Transparent";
        var backgroundKind = "image";
        if (s.BackgroundMode == "solid") backgroundName = "Solid color " + s.BackgroundColor;
        else if (s.BackgroundMode == "custom")
        {
            if (background != null)
            {
                backgroundUrl = "/media/background?v=" + File.GetLastWriteTimeUtc(background).Ticks;
                backgroundName = Path.GetFileName(background);
                backgroundKind = Path.GetExtension(background).Equals(".webm", StringComparison.OrdinalIgnoreCase) ? "video" : "image";
            }
            else backgroundName = "Custom background not set — transparent";
        }

        return new AppState
        {
            Version = AppUtil.DisplayVersion,
            Track = t,
            DisplayText = display,
            Settings = s,
            ImageURL = imageUrl,
            ImageName = imageName,
            MediaKind = mediaKind,
            BackgroundURL = backgroundUrl,
            BackgroundName = backgroundName,
            BackgroundKind = backgroundKind,
            Visible = visible,
            UpdatedAt = u,
            Detector = d,
            OverlayFPS = fps,
            OverlayFrameMS = frame,
            OverlayMetricsAt = metricsAt,
            Fonts = ListFonts(),
            Profiles = ListProfiles(),
            Diagnostics = new MediaDiagnostics
            {
                Found = t.Found,
                Source = t.Source,
                Status = t.Status,
                HasTimeline = t.DurationMS > 0,
                PositionMS = t.PositionMS,
                DurationMS = t.DurationMS,
                SampleAgeMS = t.SampledAtMS <= 0 ? 0 : Math.Max(0, AppUtil.NowMS() - t.SampledAtMS),
                Detector = d,
                DataDirectory = DataDir,
                OverlayAddress = "http://127.0.0.1:17891/overlay"
            }
        };
    }

    public string? CurrentCustomImagePath() => ResolveMediaFile(SettingsSnapshot().CustomImage, "custom_now_playing");
    public string? CurrentBackgroundPath() => ResolveMediaFile(SettingsSnapshot().CustomBackground, "custom_background");

    public async Task SetCustomImageAsync(string fileName, CancellationToken ct = default)
    {
        var s = SettingsSnapshot();
        s.CustomImage = Path.GetFileName(fileName);
        s.ImageMode = "custom";
        await ApplySettingsAsync(s, ct);
    }

    public async Task RemoveCustomImageAsync(CancellationToken ct = default)
    {
        foreach (var file in Directory.EnumerateFiles(MediaDir, "custom_now_playing.*")) TryDelete(file);
        var s = SettingsSnapshot(); s.CustomImage = ""; if (s.ImageMode == "custom") s.ImageMode = "fallback";
        await ApplySettingsAsync(s, ct);
    }

    public async Task SetBackgroundAsync(string fileName, CancellationToken ct = default)
    {
        var s = SettingsSnapshot(); s.CustomBackground = Path.GetFileName(fileName); s.BackgroundMode = "custom";
        await ApplySettingsAsync(s, ct);
    }

    public async Task RemoveBackgroundAsync(CancellationToken ct = default)
    {
        foreach (var file in Directory.EnumerateFiles(MediaDir, "custom_background.*")) TryDelete(file);
        var s = SettingsSnapshot(); s.CustomBackground = ""; if (s.BackgroundMode == "custom") s.BackgroundMode = "transparent";
        await ApplySettingsAsync(s, ct);
    }

    public List<FontInfo> ListFonts()
    {
        Directory.CreateDirectory(FontDir);
        return Directory.EnumerateFiles(FontDir)
            .Where(p => new[] { ".ttf", ".otf", ".woff", ".woff2" }.Contains(Path.GetExtension(p).ToLowerInvariant()))
            .OrderBy(p => Path.GetFileName(p), StringComparer.OrdinalIgnoreCase)
            .Select(p =>
            {
                var name = Path.GetFileName(p);
                return new FontInfo
                {
                    ID = name,
                    Name = Path.GetFileNameWithoutExtension(name),
                    Family = "NPF_" + AppUtil.SanitizeFontBase(name),
                    URL = $"/font?name={Uri.EscapeDataString(name)}&v={File.GetLastWriteTimeUtc(p).Ticks}"
                };
            }).ToList();
    }

    public string? FontPathForFamily(string family)
    {
        var f = ListFonts().FirstOrDefault(x => x.Family == family);
        return f == null ? null : Path.Combine(FontDir, Path.GetFileName(f.ID));
    }

    public List<ProfileInfo> ListProfiles()
    {
        Directory.CreateDirectory(ProfileDir);
        var def = SettingsSnapshot().DefaultProfile;
        return Directory.EnumerateDirectories(ProfileDir)
            .Select(d => new { Dir = d, File = Path.Combine(d, "profile.json") })
            .Where(x => File.Exists(x.File))
            .Select(x => new ProfileInfo { Name = Path.GetFileName(x.Dir), ModifiedAt = new DateTimeOffset(File.GetLastWriteTimeUtc(x.File)).ToUnixTimeMilliseconds(), Default = Path.GetFileName(x.Dir).Equals(def, StringComparison.OrdinalIgnoreCase) })
            .OrderBy(x => x.Name, StringComparer.OrdinalIgnoreCase).ToList();
    }

    public static string RenderText(Track t, Settings s)
    {
        if (!t.Found || (string.IsNullOrWhiteSpace(t.Artist) && string.IsNullOrWhiteSpace(t.Title)))
            return s.ShowWhenIdle ? ApplyTemplate(s.Template, "Artist Name", "Song Title") : "";
        return ApplyTemplate(s.Template, t.Artist, t.Title);
    }

    public static string ApplyTemplate(string template, string artist, string title) => (template ?? "")
        .Replace("{artist}", artist).Replace("{title}", title).Replace("{song}", title)
        .Replace("{Artist}", artist).Replace("{Title}", title);

    public static void NormalizeSettings(Settings s)
    {
        s.SchemaVersion = Math.Max(69, s.SchemaVersion);
        if (s.Format != "song_artist") s.Format = "artist_song";
        s.Template = (s.Template ?? "").Trim();
        if (s.Template.Length == 0) s.Template = s.Format == "song_artist" ? "{title} • {artist}" : "{artist} - {title}";
        if (s.Template.Length > 512) s.Template = s.Template[..512];
        s.CanvasWidth = AppUtil.Clamp(s.CanvasWidth < 100 ? 900 : s.CanvasWidth, 100, 7680);
        s.CanvasHeight = AppUtil.Clamp(s.CanvasHeight < 50 ? 180 : s.CanvasHeight, 50, 4320);
        s.TextX = AppUtil.Clamp(s.TextX, -7680, 7680); s.TextY = AppUtil.Clamp(s.TextY, -7680, 7680);
        s.ImageX = AppUtil.Clamp(s.ImageX, -7680, 7680); s.ImageY = AppUtil.Clamp(s.ImageY, -7680, 7680);
        s.ProgressX = AppUtil.Clamp(s.ProgressX, -7680, 7680); s.ProgressY = AppUtil.Clamp(s.ProgressY, -7680, 7680);
        s.TimeX = AppUtil.Clamp(s.TimeX, -7680, 7680); s.TimeY = AppUtil.Clamp(s.TimeY, -7680, 7680);
        s.TextWidth = AppUtil.Clamp(s.TextWidth < 40 ? 680 : s.TextWidth, 40, 7680);
        s.TextSize = AppUtil.Clamp(s.TextSize < 8 ? 40 : s.TextSize, 8, 200);
        if (s.TextWeight < 100 || s.TextWeight > 900) s.TextWeight = 700;
        s.TextFont = string.IsNullOrWhiteSpace(s.TextFont) || s.TextFont.Length > 128 ? "Segoe UI" : s.TextFont.Trim();
        s.TextColor = AppUtil.NormalizeColor(s.TextColor, "#FFFFFF");
        if (s.TextAlign is not ("center" or "right")) s.TextAlign = "left";
        if (s.TextLineHeight < .7 || s.TextLineHeight > 3) s.TextLineHeight = 1.15;
        if (s.ImageMode is not ("custom" or "fallback")) s.ImageMode = "fallback";
        s.ImageWidth = AppUtil.Clamp(s.ImageWidth < 1 ? 150 : s.ImageWidth, 1, 7680); s.ImageHeight = AppUtil.Clamp(s.ImageHeight < 1 ? 150 : s.ImageHeight, 1, 4320);
        s.ImageOpacity = AppUtil.Clamp(s.ImageOpacity, 0, 100);
        if (s.MediaFit is not ("contain" or "fill")) s.MediaFit = "cover";
        s.MediaPositionX = AppUtil.Clamp(s.MediaPositionX, 0, 100); s.MediaPositionY = AppUtil.Clamp(s.MediaPositionY, 0, 100);
        if (s.MediaZoom < 50 || s.MediaZoom > 250) s.MediaZoom = 100;
        s.MediaRadius = AppUtil.Clamp(s.MediaRadius, 0, 100);
        if (s.MediaBrightness < 25 || s.MediaBrightness > 200) s.MediaBrightness = 100;
        if (s.MediaContrast < 25 || s.MediaContrast > 200) s.MediaContrast = 105;
        if (s.MediaSaturation < 0 || s.MediaSaturation > 250) s.MediaSaturation = 105;
        s.MediaBorderWidth = AppUtil.Clamp(s.MediaBorderWidth, 0, 20); s.MediaBorderColor = AppUtil.NormalizeColor(s.MediaBorderColor, "#3AA7FF");
        s.MediaGlowColor = AppUtil.NormalizeColor(s.MediaGlowColor, "#3AA7FF"); s.MediaGlowSize = AppUtil.Clamp(s.MediaGlowSize, 0, 80);
        s.ArtworkAnimationMS = AppUtil.Clamp(s.ArtworkAnimationMS, 800, 20000); s.OverlayAnimationMS = AppUtil.Clamp(s.OverlayAnimationMS, 800, 20000);
        if (!new[] { "float", "pulse", "breathe", "tilt", "slow-rotate", "none" }.Contains(s.ArtworkAnimation)) s.ArtworkAnimation = "none";
        if (!new[] { "float", "pulse", "breathe", "shimmer", "none" }.Contains(s.TextAnimation)) s.TextAnimation = "none";
        if (!new[] { "soft-glow", "neon", "outline", "none" }.Contains(s.TextEffect)) s.TextEffect = "none";
        if (!new[] { "float", "pulse", "breathe", "none" }.Contains(s.OverlayAnimation)) s.OverlayAnimation = "none";
        if (!new[] { "slow-zoom", "pan-left", "pan-right", "none" }.Contains(s.BackgroundMotion)) s.BackgroundMotion = "none";
        if (s.BackgroundMode is not ("solid" or "custom")) s.BackgroundMode = "transparent";
        s.BackgroundColor = AppUtil.NormalizeColor(s.BackgroundColor, "#000000"); s.BackgroundOpacity = AppUtil.Clamp(s.BackgroundOpacity, 0, 100);
        if (s.BackgroundFit is not ("contain" or "fill")) s.BackgroundFit = "cover";
        s.BackgroundPositionX = AppUtil.Clamp(s.BackgroundPositionX, 0, 100); s.BackgroundPositionY = AppUtil.Clamp(s.BackgroundPositionY, 0, 100);
        if (s.BackgroundZoom < 50 || s.BackgroundZoom > 250) s.BackgroundZoom = 100;
        s.BackgroundRadius = AppUtil.Clamp(s.BackgroundRadius, 0, 200);
        if (s.BackgroundBrightness < 25 || s.BackgroundBrightness > 200) s.BackgroundBrightness = 100;
        if (s.BackgroundContrast < 25 || s.BackgroundContrast > 200) s.BackgroundContrast = 100;
        if (s.BackgroundSaturation < 0 || s.BackgroundSaturation > 250) s.BackgroundSaturation = 100;
        s.BackgroundBlur = AppUtil.Clamp(s.BackgroundBlur, 0, 40);
        if (s.ProgressMode != "remaining") s.ProgressMode = "elapsed";
        s.ProgressWidth = AppUtil.Clamp(s.ProgressWidth < 20 ? 680 : s.ProgressWidth, 20, 7680); s.ProgressHeight = AppUtil.Clamp(s.ProgressHeight < 2 ? 10 : s.ProgressHeight, 2, 80);
        s.ProgressColor = AppUtil.NormalizeColor(s.ProgressColor, "#3AA7FF"); s.ProgressTrackColor = AppUtil.NormalizeColor(s.ProgressTrackColor, "#26384F"); s.ProgressRadius = AppUtil.Clamp(s.ProgressRadius, 0, 40);
        s.ProgressTextColor = AppUtil.NormalizeColor(s.ProgressTextColor, "#DCEBFF"); if (s.ProgressTextSize < 8 || s.ProgressTextSize > 80) s.ProgressTextSize = 13;
        s.TimeWidth = AppUtil.Clamp(s.TimeWidth < 20 ? 680 : s.TimeWidth, 20, 7680); if (s.TimeAlign is not ("left" or "center")) s.TimeAlign = "right";
        if (!new[] { "square", "pill", "glow", "segmented", "gradient", "rounded" }.Contains(s.ProgressStyle)) s.ProgressStyle = "rounded";
        if (!new[] { "none", "slide-left", "slide-right", "slide-up", "slide-down", "scale", "zoom-in", "zoom-out", "blur", "flip", "fade" }.Contains(s.TransitionStyle)) s.TransitionStyle = "fade";
        s.TransitionMS = AppUtil.Clamp(s.TransitionMS, 0, 5000);
        if (!new[] { "linear", "ease", "ease-in", "ease-out", "ease-in-out", "snappy", "spring", "smooth" }.Contains(s.TransitionEasing)) s.TransitionEasing = "smooth";
        if (s.GridSize < 1 || s.GridSize > 200) s.GridSize = 10;
        s.DesignerTheme = s.DesignerTheme switch { "midnight" => "blue", "violet" => "purple", "blue" or "red" or "purple" or "green" or "pink" => s.DesignerTheme, _ => "blue" };
        if (s.StartupPage != "last_module") s.StartupPage = "home";
        if (!new[] { "chat-overlay", "stream-settings", "connections", "countdown-pro", "now-playing" }.Contains(s.LastModule)) s.LastModule = "now-playing";
        s.DefaultProfile = AppUtil.SanitizeProfileName(s.DefaultProfile);
        if (s.MediaSourceMode is not ("any" or "custom")) s.MediaSourceMode = "spotify";
        s.MediaSourceInclude = (s.MediaSourceInclude ?? "").Trim(); s.MediaSourceExclude = (s.MediaSourceExclude ?? "").Trim();
        if (s.MediaSourceInclude.Length > 512) s.MediaSourceInclude = s.MediaSourceInclude[..512]; if (s.MediaSourceExclude.Length > 512) s.MediaSourceExclude = s.MediaSourceExclude[..512];
    }

    private void FindExistingMedia()
    {
        var s = settings;
        if (ResolveMediaFile(s.CustomImage, "custom_now_playing") is string custom) s.CustomImage = Path.GetFileName(custom); else s.CustomImage = "";
        if (ResolveMediaFile(s.CustomBackground, "custom_background") is string bg) s.CustomBackground = Path.GetFileName(bg); else s.CustomBackground = "";
    }

    private string? ResolveMediaFile(string? storedName, string prefix)
    {
        if (!Directory.Exists(MediaDir)) return null;
        if (!string.IsNullOrWhiteSpace(storedName))
        {
            var path = Path.Combine(MediaDir, Path.GetFileName(storedName));
            if (File.Exists(path)) return path;
        }
        return Directory.EnumerateFiles(MediaDir).FirstOrDefault(p => Path.GetFileNameWithoutExtension(p).Equals(prefix, StringComparison.OrdinalIgnoreCase) && new[] { ".png", ".jpg", ".jpeg", ".webp", ".gif", ".webm" }.Contains(Path.GetExtension(p).ToLowerInvariant()));
    }

    private void WriteLegacyOutput()
    {
        Track t; Settings s;
        lock (gate) { t = Clone(track); s = Clone(settings); }
        var text = RenderText(t, s); if (!t.Found && !s.ShowWhenIdle) text = "";
        lock (outputGate)
        {
            if (text == lastOutput && File.Exists(OutputPath)) return;
            try { File.WriteAllText(OutputPath, text); lastOutput = text; } catch { }
        }
    }

    private void MigrateLegacyPortableData()
    {
        CopyIfMissing(Path.Combine(ExeDir, "settings.json"), SettingsPath);
        CopyIfMissing(Path.Combine(ExeDir, "now_playing.txt"), OutputPath);
        var oldAssets = Path.Combine(ExeDir, "now_playing_assets");
        if (Directory.Exists(oldAssets)) foreach (var src in Directory.EnumerateFiles(oldAssets, "*", SearchOption.AllDirectories))
        {
            var rel = Path.GetRelativePath(oldAssets, src); CopyIfMissing(src, Path.Combine(MediaDir, rel));
        }
    }

    private static void CopyIfMissing(string src, string dst)
    {
        try { if (File.Exists(src) && !File.Exists(dst)) { Directory.CreateDirectory(Path.GetDirectoryName(dst)!); File.Copy(src, dst); } } catch { }
    }
    private static void TryDelete(string path) { try { File.Delete(path); } catch { } }
    private static bool TrackEquals(Track a, Track b) => JsonSerializer.Serialize(a, AppUtil.Json) == JsonSerializer.Serialize(b, AppUtil.Json);
    private static T Clone<T>(T value) => JsonSerializer.Deserialize<T>(JsonSerializer.Serialize(value, AppUtil.Json), AppUtil.Json)!;
}
