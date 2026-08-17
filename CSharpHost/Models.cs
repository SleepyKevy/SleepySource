using System.Text.Json.Serialization;

namespace SleepySource;

internal sealed class Settings
{
    public int SchemaVersion { get; set; } = 69;
    public string Format { get; set; } = "artist_song";
    public string Template { get; set; } = "{artist} - {title}";
    public int CanvasWidth { get; set; } = 900;
    public int CanvasHeight { get; set; } = 180;
    public int TextX { get; set; } = 190;
    public int TextY { get; set; } = 58;
    public int TextWidth { get; set; } = 680;
    public int TextSize { get; set; } = 40;
    public int TextWeight { get; set; } = 700;
    public string TextFont { get; set; } = "Segoe UI";
    public string TextColor { get; set; } = "#FFFFFF";
    public string TextAlign { get; set; } = "left";
    public double TextLineHeight { get; set; } = 1.15;
    public bool TextShadow { get; set; }
    public string ImageMode { get; set; } = "fallback";
    public int ImageX { get; set; } = 15;
    public int ImageY { get; set; } = 15;
    public int ImageWidth { get; set; } = 150;
    public int ImageHeight { get; set; } = 150;
    public int ImageOpacity { get; set; } = 100;
    public string MediaFit { get; set; } = "cover";
    public int MediaPositionX { get; set; } = 50;
    public int MediaPositionY { get; set; } = 50;
    public int MediaZoom { get; set; } = 100;
    public int MediaRadius { get; set; } = 12;
    public bool MediaShadow { get; set; }
    public int MediaBrightness { get; set; } = 100;
    public int MediaContrast { get; set; } = 105;
    public int MediaSaturation { get; set; } = 105;
    public int MediaBorderWidth { get; set; }
    public string MediaBorderColor { get; set; } = "#3AA7FF";
    public bool MediaGlow { get; set; }
    public string MediaGlowColor { get; set; } = "#3AA7FF";
    public int MediaGlowSize { get; set; } = 18;
    public string ArtworkAnimation { get; set; } = "none";
    public int ArtworkAnimationMS { get; set; } = 5000;
    public string TextAnimation { get; set; } = "none";
    public string TextEffect { get; set; } = "none";
    public string OverlayAnimation { get; set; } = "none";
    public int OverlayAnimationMS { get; set; } = 6000;
    public string BackgroundMotion { get; set; } = "none";
    public string CustomImage { get; set; } = "";
    public bool ShowProgress { get; set; } = true;
    public string ProgressMode { get; set; } = "elapsed";
    public int ProgressX { get; set; } = 190;
    public int ProgressY { get; set; } = 126;
    public int ProgressWidth { get; set; } = 680;
    public int ProgressHeight { get; set; } = 10;
    public string ProgressColor { get; set; } = "#3AA7FF";
    public string ProgressTrackColor { get; set; } = "#26384F";
    public int ProgressRadius { get; set; } = 6;
    public bool ShowRemainingTime { get; set; } = true;
    public string ProgressTextColor { get; set; } = "#DCEBFF";
    public int ProgressTextSize { get; set; } = 13;
    public int TimeX { get; set; } = 190;
    public int TimeY { get; set; } = 140;
    public int TimeWidth { get; set; } = 680;
    public string TimeAlign { get; set; } = "right";
    public string BackgroundMode { get; set; } = "transparent";
    public string BackgroundColor { get; set; } = "#000000";
    public int BackgroundOpacity { get; set; } = 100;
    public string BackgroundFit { get; set; } = "cover";
    public int BackgroundPositionX { get; set; } = 50;
    public int BackgroundPositionY { get; set; } = 50;
    public int BackgroundZoom { get; set; } = 100;
    public int BackgroundRadius { get; set; }
    public int BackgroundBrightness { get; set; } = 100;
    public int BackgroundContrast { get; set; } = 100;
    public int BackgroundSaturation { get; set; } = 100;
    public int BackgroundBlur { get; set; }
    public string CustomBackground { get; set; } = "";
    public bool HideWhenPaused { get; set; }
    public bool ShowWhenIdle { get; set; }
    public string ProgressStyle { get; set; } = "rounded";
    public string TransitionStyle { get; set; } = "fade";
    public int TransitionMS { get; set; } = 300;
    public string TransitionEasing { get; set; } = "smooth";
    public bool SnapEnabled { get; set; } = true;
    public int GridSize { get; set; } = 10;
    public bool OnboardingComplete { get; set; }
    public string DesignerTheme { get; set; } = "blue";
    public string StartupPage { get; set; } = "home";
    public string LastModule { get; set; } = "now-playing";
    public string DefaultProfile { get; set; } = "";
    public string MediaSourceMode { get; set; } = "spotify";
    public string MediaSourceInclude { get; set; } = "";
    public string MediaSourceExclude { get; set; } = "";
}

internal sealed class Track
{
    public bool Found { get; set; }
    public string Artist { get; set; } = "";
    public string Title { get; set; } = "";
    public string Status { get; set; } = "stopped";
    public string Source { get; set; } = "";
    public long PositionMS { get; set; }
    public long DurationMS { get; set; }
    public string ArtStatus { get; set; } = "";
    public long SampledAtMS { get; set; }
}

internal sealed class FontInfo
{
    public string ID { get; set; } = "";
    public string Name { get; set; } = "";
    public string Family { get; set; } = "";
    public string URL { get; set; } = "";
}

internal sealed class ProfileInfo
{
    public string Name { get; set; } = "";
    public long ModifiedAt { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)]
    public bool Default { get; set; }
}

internal sealed class MediaDiagnostics
{
    public bool Found { get; set; }
    public string Source { get; set; } = "";
    public string Status { get; set; } = "";
    public bool HasTimeline { get; set; }
    public long PositionMS { get; set; }
    public long DurationMS { get; set; }
    public long SampleAgeMS { get; set; }
    public string Detector { get; set; } = "";
    public string DataDirectory { get; set; } = "";
    public string OverlayAddress { get; set; } = "http://127.0.0.1:17891/overlay";
}

internal sealed class AppState
{
    public string Version { get; set; } = "1.0 Beta";
    public Track Track { get; set; } = new();
    public string DisplayText { get; set; } = "";
    public Settings Settings { get; set; } = new();
    public string ImageURL { get; set; } = "/assets/default.png";
    public string ImageName { get; set; } = "Default artwork";
    public string MediaKind { get; set; } = "image";
    public string BackgroundURL { get; set; } = "";
    public string BackgroundName { get; set; } = "";
    public string BackgroundKind { get; set; } = "";
    public bool Visible { get; set; } = true;
    public long UpdatedAt { get; set; }
    public string Detector { get; set; } = "Windows media session detector";
    public double OverlayFPS { get; set; }
    public double OverlayFrameMS { get; set; }
    public long OverlayMetricsAt { get; set; }
    public List<FontInfo> Fonts { get; set; } = [];
    public List<ProfileInfo> Profiles { get; set; } = [];
    public MediaDiagnostics Diagnostics { get; set; } = new();
}

internal sealed class ChatSettings
{
    public int SchemaVersion { get; set; } = 6;
    public int CanvasWidth { get; set; } = 900;
    public int CanvasHeight { get; set; } = 600;
    public int BoxX { get; set; } = 30;
    public int BoxY { get; set; } = 30;
    public int BoxWidth { get; set; } = 520;
    public int BoxHeight { get; set; } = 520;
    public string FontFamily { get; set; } = "Segoe UI";
    public int FontSize { get; set; } = 24;
    public int UsernameSize { get; set; } = 22;
    public string MessageColor { get; set; } = "#FFFFFF";
    public string UsernameColor { get; set; } = "#55B7FF";
    public string BackgroundColor { get; set; } = "#07111F";
    public int BackgroundOpacity { get; set; } = 72;
    public bool BoxBackgroundTransparent { get; set; }
    public string BorderColor { get; set; } = "#2F78B7";
    public int BorderWidth { get; set; } = 1;
    public int Radius { get; set; } = 14;
    public int Padding { get; set; } = 16;
    public int MessageGap { get; set; } = 10;
    public int EmoteSize { get; set; } = 32;
    public int BadgeSize { get; set; } = 20;
    public int MaxMessages { get; set; } = 12;
    public bool CompactMode { get; set; } = true;
    public string MessageBackgroundColor { get; set; } = "#07111F";
    public int MessageBackgroundOpacity { get; set; } = 22;
    public bool MessageBackgroundTransparent { get; set; }
    public bool TextShadow { get; set; } = true;
    public bool UseKickUsernameColor { get; set; } = true;
    public bool ShowBadges { get; set; } = true;
    public bool ShowTimestamps { get; set; }
    public bool ShowAvatars { get; set; }
    public bool HideCommands { get; set; }
    public string Direction { get; set; } = "bottom-up";
    public string Animation { get; set; } = "slide-up";
    public int AnimationMS { get; set; } = 240;
    public string Theme { get; set; } = "midnight";
    public string AnimationEasing { get; set; } = "smooth";
    public int MessageBorderWidth { get; set; }
    public string MessageBorderColor { get; set; } = "#2F78B7";
    public int MessageRadius { get; set; } = 9;
    public int BoxBlur { get; set; } = 2;
    public int UsernameWeight { get; set; } = 800;
    [JsonPropertyName("seventv_enabled")]
    public bool SevenTVEnabled { get; set; } = true;
    public bool RememberKickLogin { get; set; } = true;
    public string KickChannel { get; set; } = "";
    [JsonPropertyName("seventv_emote_set_id")]
    public string SevenTVEmoteSetID { get; set; } = "";
}

internal sealed class ChatBadge
{
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Text { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Type { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public int Count { get; set; }
}

internal sealed class ChatMessage
{
    public string ID { get; set; } = "";
    public string UserID { get; set; } = "";
    public string Username { get; set; } = "";
    public string Color { get; set; } = "#55B7FF";
    public string Text { get; set; } = "";
    public List<string> Badges { get; set; } = [];
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public List<ChatBadge>? BadgeDetails { get; set; }
    public string AvatarURL { get; set; } = "";
    public long CreatedAt { get; set; }
    public bool IsMod { get; set; }
}

internal sealed class ChatState
{
    public ChatSettings Settings { get; set; } = new();
    public List<ChatMessage> Messages { get; set; } = [];
    public long UpdatedAt { get; set; }
    public bool AuthReady { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string AuthMode { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string ConnectedChannel { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string BroadcasterUserID { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long TokenExpiresAt { get; set; }
    public bool LiveChatConnected { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string LiveChatStatus { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string ChatroomID { get; set; } = "";
    public bool WebhookSubscribed { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string WebhookSubscriptionID { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long WebhookLastEventAt { get; set; }
    public long WebhookRequestCount { get; set; }
    public long WebhookVerifiedCount { get; set; }
    public long WebhookAcceptedCount { get; set; }
    public long WebhookRejectedCount { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long WebhookLastRequestAt { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string WebhookLastEventType { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string WebhookLastError { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string SavedClientID { get; set; } = "";
    public bool CredentialsSaved { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string CredentialStorage { get; set; } = "";
}

internal sealed class CountdownSettings
{
    public int SchemaVersion { get; set; } = 2;
    public string Mode { get; set; } = "countdown";
    public int Hours { get; set; }
    public int Minutes { get; set; } = 5;
    public int Seconds { get; set; }
    public string Format { get; set; } = "auto";
    public string CustomFormat { get; set; } = "{hh}:{mm}:{ss}";
    public string Prefix { get; set; } = "";
    public string Suffix { get; set; } = "";
    public string FinishedText { get; set; } = "STARTING NOW";
    public bool BlankOnFinish { get; set; }
    public bool Loop { get; set; }
    public bool Overtime { get; set; }
    public string StartBehavior { get; set; } = "manual";
    public bool RestartOnLoad { get; set; }
    public bool ResetOnUnload { get; set; }
    public int CanvasWidth { get; set; } = 900;
    public int CanvasHeight { get; set; } = 220;
    public bool CanvasTransparent { get; set; } = true;
    public string CanvasColor { get; set; } = "#000000";
    public int CanvasOpacity { get; set; } = 100;
    public int TimerX { get; set; } = 50;
    public int TimerY { get; set; } = 45;
    public int TimerWidth { get; set; } = 800;
    public string FontFamily { get; set; } = "Segoe UI";
    public int FontSize { get; set; } = 96;
    public int FontWeight { get; set; } = 700;
    public string TextColor { get; set; } = "#FFFFFF";
    public int TextOpacity { get; set; } = 100;
    public string Align { get; set; } = "center";
    public double LetterSpacing { get; set; }
    public double LineHeight { get; set; } = 1;
    public bool TextShadow { get; set; } = true;
    public bool Outline { get; set; } = true;
    public int OutlineSize { get; set; } = 3;
    public string OutlineColor { get; set; } = "#000000";
    public int OutlineOpacity { get; set; } = 100;
    public bool PanelEnabled { get; set; }
    public string PanelColor { get; set; } = "#07111F";
    public int PanelOpacity { get; set; } = 70;
    public int PanelRadius { get; set; } = 18;
    public int PanelPadding { get; set; } = 18;
    public int BorderWidth { get; set; }
    public string BorderColor { get; set; } = "#3AA7FF";
    public int BorderOpacity { get; set; } = 100;
    public string TimerAnimation { get; set; } = "none";
    public string TickAnimation { get; set; } = "none";
    public string PanelAnimation { get; set; } = "none";
    public string OverlayAnimation { get; set; } = "none";
    public int AnimationMS { get; set; } = 1800;
}

internal sealed class CountdownState
{
    public CountdownSettings Settings { get; set; } = new();
    public long CurrentMS { get; set; }
    public long DurationMS { get; set; }
    public bool Running { get; set; }
    public bool Paused { get; set; }
    public bool Finished { get; set; }
    public bool HasStarted { get; set; }
    public string DisplayText { get; set; } = "";
    public long ServerNowMS { get; set; }
    public long UpdatedAtMS { get; set; }
    public string OverlayURL { get; set; } = "http://127.0.0.1:17891/countdown";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<FontInfo>? Fonts { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public List<ProfileInfo>? Profiles { get; set; }
}

internal sealed class AlertStyle
{
    public bool Enabled { get; set; } = true;
    public string DisplayMode { get; set; } = "card";
    public bool ShowTitle { get; set; } = true;
    public bool ShowMessage { get; set; } = true;
    public string TitleTemplate { get; set; } = "";
    public string MessageTemplate { get; set; } = "";
    public int DurationMS { get; set; } = 4200;
    public string EnterAnimation { get; set; } = "pop";
    public string ExitAnimation { get; set; } = "fade";
    public int EnterDurationMS { get; set; } = 420;
    public int ExitDurationMS { get; set; } = 320;
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Animation { get; set; } = "pop";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Layout { get; set; } = "card";
    public int X { get; set; } = 610;
    public int Y { get; set; } = 365;
    public int Width { get; set; } = 700;
    public int Height { get; set; } = 350;
    public bool SnapEnabled { get; set; } = true;
    public int MediaX { get; set; } = 34;
    public int MediaY { get; set; } = 90;
    public int MediaWidth { get; set; } = 170;
    public int MediaHeight { get; set; } = 170;
    public string MediaFit { get; set; } = "contain";
    public int MediaOpacity { get; set; } = 100;
    public int MediaRotation { get; set; }
    public bool MediaFlipHorizontal { get; set; }
    public bool MediaFlipVertical { get; set; }
    public bool MediaAboveText { get; set; }
    public int TitleX { get; set; } = 235;
    public int TitleY { get; set; } = 105;
    public int TitleWidth { get; set; } = 425;
    public int TitleHeight { get; set; } = 82;
    public string TitleFontFamily { get; set; } = "Segoe UI";
    public int TitleSize { get; set; } = 42;
    public int TitleWeight { get; set; } = 900;
    public string TitleColor { get; set; } = "#FFFFFF";
    public string TitleAlign { get; set; } = "left";
    public string TitleOutlineColor { get; set; } = "#000000";
    public int TitleOutlineWidth { get; set; }
    public int TitleLetterSpacing { get; set; }
    public int TitleLineHeight { get; set; } = 108;
    public bool TitleShadow { get; set; } = true;
    public int MessageX { get; set; } = 235;
    public int MessageY { get; set; } = 175;
    public int MessageWidth { get; set; } = 425;
    public int MessageHeight { get; set; } = 78;
    public string MessageFontFamily { get; set; } = "Segoe UI";
    public int MessageSize { get; set; } = 24;
    public int MessageWeight { get; set; } = 650;
    public string MessageColor { get; set; } = "#FFFFFF";
    public string MessageAlign { get; set; } = "left";
    public string MessageOutlineColor { get; set; } = "#000000";
    public int MessageOutlineWidth { get; set; }
    public int MessageLetterSpacing { get; set; }
    public int MessageLineHeight { get; set; } = 128;
    public bool MessageShadow { get; set; } = true;
    public string BackgroundColor { get; set; } = "#07111F";
    public int BackgroundOpacity { get; set; } = 92;
    public string AccentColor { get; set; } = "#3AA7FF";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string TextColor { get; set; } = "#FFFFFF";
    public int Radius { get; set; } = 24;
    public int BorderWidth { get; set; } = 2;
    public bool Shadow { get; set; } = true;
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string VisualFile { get; set; } = null!;
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long VisualUpdatedAt { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string SoundFile { get; set; } = null!;
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long SoundUpdatedAt { get; set; }
    public int SoundVolume { get; set; } = 75;
    public int SoundDelayMS { get; set; }
}

internal sealed class AlertSettings
{
    public int SchemaVersion { get; set; } = 2;
    public int CanvasWidth { get; set; } = 1920;
    public int CanvasHeight { get; set; } = 1080;
    public int QueueLimit { get; set; } = 40;
    public Dictionary<string, AlertStyle> Types { get; set; } = new(StringComparer.Ordinal);
}

internal class AlertEvent
{
    public string ID { get; set; } = "";
    public string Type { get; set; } = "";
    public string Source { get; set; } = "";
    public string Username { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public int Amount { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public int Count { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public int Months { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Tier { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string GiftName { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string RewardTitle { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string UserInput { get; set; } = "";
    public long CreatedAtMS { get; set; }
}

internal sealed class AlertPresentation : AlertEvent
{
    public AlertStyle Style { get; set; } = new();
    public string Title { get; set; } = "";
    public string Message { get; set; } = "";
    public long StartedAtMS { get; set; }
    public long EndsAtMS { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string VisualURL { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string SoundURL { get; set; } = "";
}

internal sealed class AlertState
{
    public AlertSettings Settings { get; set; } = new();
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public AlertPresentation? Current { get; set; }
    public List<AlertEvent> Queue { get; set; } = [];
    public int QueueDepth { get; set; }
    public long Dropped { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] public AlertEvent? LastEvent { get; set; }
    public string OverlayURL { get; set; } = "http://127.0.0.1:17891/alerts";
    public long UpdatedAtMS { get; set; }
}

internal sealed class HealthCheck
{
    public string ID { get; set; } = "";
    public string Group { get; set; } = "";
    public string Name { get; set; } = "";
    public string Status { get; set; } = "";
    public string Message { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string RepairAction { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string RepairLabel { get; set; } = "";
}

internal sealed class HealthReport
{
    public string Version { get; set; } = "1.0 Beta";
    public long CheckedAt { get; set; }
    public string OverallStatus { get; set; } = "pass";
    public string Summary { get; set; } = "";
    public List<HealthCheck> Checks { get; set; } = [];
}

internal sealed class KickUserAuthState
{
    public bool Authorized { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string Scope { get; set; } = "";
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public long ExpiresAt { get; set; }
    public string RedirectURI { get; set; } = "http://127.0.0.1:17891/oauth/kick/callback";
    public bool Pending { get; set; }
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingDefault)] public string LastError { get; set; } = "";
    public bool HasAppCredentials { get; set; }
}

internal sealed class CloudflareTunnelState
{
    public bool Running { get; set; }
    public string Status { get; set; } = "stopped";
    public string PublicURL { get; set; } = "";
    public string WebhookURL { get; set; } = "";
    public string LastError { get; set; } = "";
    public string Executable { get; set; } = "";
    public long StartedAt { get; set; }
}
