using Windows.Media.Control;

namespace SleepySource;

internal sealed class MediaSessionService : IDisposable
{
    private readonly CoreStateService core;
    private CancellationTokenSource? stopCts;
    private Task? worker;

    public MediaSessionService(CoreStateService core) => this.core = core;

    public Task StartAsync(CancellationToken startupToken = default)
    {
        if (worker is not null) return Task.CompletedTask;
        stopCts = CancellationTokenSource.CreateLinkedTokenSource(startupToken);
        worker = Task.Run(() => RunAsync(stopCts.Token), CancellationToken.None);
        return Task.CompletedTask;
    }

    public void Stop()
    {
        try { stopCts?.Cancel(); } catch { }
        try { worker?.Wait(TimeSpan.FromSeconds(2)); } catch { }
    }

    private async Task RunAsync(CancellationToken ct)
    {
        GlobalSystemMediaTransportControlsSessionManager? manager = null;
        try
        {
            manager = await GlobalSystemMediaTransportControlsSessionManager.RequestAsync();
            core.SetDetectorStatus("Native Windows media-session detector ready — waiting for an eligible media session.");
        }
        catch (Exception ex)
        {
            core.SetDetectorStatus("Native Windows media-session detection unavailable: " + ex.Message);
            return;
        }

        while (!ct.IsCancellationRequested)
        {
            try
            {
                var settings = core.SettingsSnapshot();
                var selected = SelectSession(manager, settings);
                if (selected is null)
                {
                    core.UpdateTrack(new Track(), "Native Windows media-session detector ready — waiting for an eligible media session.");
                }
                else
                {
                    var session = selected.Value.Session;
                    var source = selected.Value.Source;
                    var playback = session.GetPlaybackInfo();
                    var props = await session.TryGetMediaPropertiesAsync();
                    var timeline = session.GetTimelineProperties();
                    var title = (props?.Title ?? "").Trim();
                    var artist = (props?.Artist ?? "").Trim();
                    if (artist.Length == 0) artist = (props?.AlbumArtist ?? "").Trim();
                    var duration = timeline.EndTime > timeline.StartTime ? (long)(timeline.EndTime - timeline.StartTime).TotalMilliseconds : 0;
                    var position = timeline.Position > timeline.StartTime ? (long)(timeline.Position - timeline.StartTime).TotalMilliseconds : 0;
                    if (position < 0) position = 0;
                    if (duration > 0 && position > duration) position = duration;
                    core.UpdateTrack(new Track
                    {
                        Found = title.Length > 0 || artist.Length > 0,
                        Artist = artist,
                        Title = title,
                        Status = playback.PlaybackStatus.ToString(),
                        Source = source,
                        PositionMS = position,
                        DurationMS = duration,
                        SampledAtMS = AppUtil.NowMS()
                    }, "Native Windows media-session detector connected.");
                }
            }
            catch (Exception ex) when (!ct.IsCancellationRequested)
            {
                core.UpdateTrack(new Track(), "Native Windows media-session detector error: " + ex.Message);
            }

            try { await Task.Delay(500, ct); }
            catch (OperationCanceledException) { break; }
        }
    }

    private static (GlobalSystemMediaTransportControlsSession Session, string Source)? SelectSession(GlobalSystemMediaTransportControlsSessionManager manager, Settings settings)
    {
        GlobalSystemMediaTransportControlsSession? chosen = null;
        string chosenSource = "";
        GlobalSystemMediaTransportControlsSessionPlaybackStatus chosenStatus = GlobalSystemMediaTransportControlsSessionPlaybackStatus.Closed;
        foreach (var session in manager.GetSessions())
        {
            var source = (session.SourceAppUserModelId ?? "").Trim();
            if (!SourceAllowed(source, settings)) continue;
            var status = session.GetPlaybackInfo().PlaybackStatus;
            if (chosen is null || (chosenStatus != GlobalSystemMediaTransportControlsSessionPlaybackStatus.Playing && status == GlobalSystemMediaTransportControlsSessionPlaybackStatus.Playing))
            {
                chosen = session;
                chosenSource = source;
                chosenStatus = status;
                if (status == GlobalSystemMediaTransportControlsSessionPlaybackStatus.Playing) break;
            }
        }
        return chosen is null ? null : (chosen, chosenSource);
    }

    private static bool SourceAllowed(string source, Settings settings)
    {
        source = source.Trim().ToLowerInvariant();
        if (source.Length == 0) return false;
        var excluded = SplitTokens(settings.MediaSourceExclude);
        if (excluded.Any(source.Contains)) return false;
        return settings.MediaSourceMode switch
        {
            "any" => true,
            "custom" => SplitTokens(settings.MediaSourceInclude) is var inc && inc.Count > 0 && inc.Any(source.Contains),
            _ => source.Contains("spotify", StringComparison.OrdinalIgnoreCase)
        };
    }

    private static List<string> SplitTokens(string? value) => (value ?? "").Split(',')
        .Select(x => x.Trim().ToLowerInvariant()).Where(x => x.Length > 0).ToList();

    public void Dispose()
    {
        Stop();
        stopCts?.Dispose();
    }
}
