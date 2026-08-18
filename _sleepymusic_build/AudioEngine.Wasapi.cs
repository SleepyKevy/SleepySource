using NAudio.Wave;

namespace SleepyMusic;

internal sealed class AudioEngine : IDisposable
{
    private readonly object sync = new();
    private WasapiOut? output;
    private WaveStream? reader;
    private bool suppressStopped;
    private float volume = 0.8f;

    public event Action? TrackEnded;
    public event Action<string>? PlaybackError;

    public string? TrackId { get; private set; }
    public bool IsPlaying { get { lock (sync) return output?.PlaybackState == PlaybackState.Playing; } }
    public bool IsLoaded { get { lock (sync) return reader is not null; } }
    public double PositionSeconds { get { lock (sync) return reader?.CurrentTime.TotalSeconds ?? 0; } }
    public double DurationSeconds { get { lock (sync) return reader?.TotalTime.TotalSeconds ?? 0; } }
    public float Volume { get { lock (sync) return volume; } }

    public void Load(string trackId, string path, bool autoplay, double startPositionSeconds = 0)
    {
        lock (sync)
        {
            DisposePlaybackUnsafe();
            try
            {
                // Decode directly from the local file through Windows Media Foundation.
                // No localhost HTTP stream or browser audio element is involved.
                var nextReader = new MediaFoundationReader(path);

                // WASAPI shared mode uses the current Windows default output device and
                // automatically adapts the decoded stream to the Windows mix format.
                var nextOutput = new WasapiOut();
                nextOutput.PlaybackStopped += OnPlaybackStopped;
                nextOutput.Init(nextReader);
                nextOutput.Volume = volume;

                reader = nextReader;
                output = nextOutput;
                TrackId = trackId;
                SeekUnsafe(startPositionSeconds);
                if (autoplay) output.Play();
            }
            catch
            {
                DisposePlaybackUnsafe();
                throw;
            }
        }
    }

    public void Play()
    {
        lock (sync)
        {
            if (output is null) return;
            output.Play();
        }
    }

    public void Pause()
    {
        lock (sync)
        {
            if (output is null) return;
            output.Pause();
        }
    }

    public void Stop(bool rewind = true)
    {
        lock (sync)
        {
            if (output is null) return;
            suppressStopped = true;
            try { output.Stop(); } finally { suppressStopped = false; }
            if (rewind) SeekUnsafe(0);
        }
    }

    public void Seek(double seconds)
    {
        lock (sync) SeekUnsafe(seconds);
    }

    public void SetVolume(double value)
    {
        lock (sync)
        {
            volume = (float)Math.Clamp(value, 0, 1);
            if (output is not null) output.Volume = volume;
        }
    }

    private void SeekUnsafe(double seconds)
    {
        if (reader is null) return;
        var max = Math.Max(0, reader.TotalTime.TotalSeconds - 0.02);
        reader.CurrentTime = TimeSpan.FromSeconds(Math.Clamp(seconds, 0, max));
    }

    private void OnPlaybackStopped(object? sender, StoppedEventArgs e)
    {
        bool ended = false;
        lock (sync)
        {
            if (e.Exception is not null)
            {
                PlaybackError?.Invoke(e.Exception.Message);
                return;
            }
            if (!suppressStopped && reader is not null)
            {
                var duration = reader.TotalTime.TotalSeconds;
                var position = reader.CurrentTime.TotalSeconds;
                ended = duration > 0 && position >= duration - 0.35;
            }
        }
        if (ended) TrackEnded?.Invoke();
    }

    private void DisposePlaybackUnsafe()
    {
        if (output is not null)
        {
            suppressStopped = true;
            try { output.Stop(); } catch { }
            suppressStopped = false;
            output.PlaybackStopped -= OnPlaybackStopped;
            output.Dispose();
            output = null;
        }

        reader?.Dispose();
        reader = null;
        TrackId = null;
    }

    public void Dispose()
    {
        lock (sync) DisposePlaybackUnsafe();
    }
}
