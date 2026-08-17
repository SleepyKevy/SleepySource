using System.Diagnostics;
using System.Net.Http;

namespace SleepySource;

internal sealed class EngineProcessManager : IDisposable
{
    private const string HealthUrl = "http://127.0.0.1:17891/api/health";
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromMilliseconds(450) };
    private Process? process;
    private bool disposed;

    public string EnginePath => Path.Combine(AppContext.BaseDirectory, "SleepySource.Engine.exe");

    public async Task StartAsync(CancellationToken cancellationToken)
    {
        if (await IsHealthyAsync(cancellationToken))
            throw new InvalidOperationException("Port 17891 is already in use by another SleepySource instance. Close the other SleepySource build before starting SleepySource 1.0 Beta.");

        if (!File.Exists(EnginePath))
            throw new FileNotFoundException("The SleepySource compatibility engine is missing.", EnginePath);

        var startInfo = new ProcessStartInfo
        {
            FileName = EnginePath,
            WorkingDirectory = AppContext.BaseDirectory,
            UseShellExecute = false,
            CreateNoWindow = true,
            WindowStyle = ProcessWindowStyle.Hidden
        };
        startInfo.ArgumentList.Add("--engine-only");
        startInfo.Environment["SLEEPYSOURCE_ENGINE_ONLY"] = "1";
        startInfo.Environment["SLEEPYSOURCE_DESKTOP_HOST"] = "csharp-1.0-beta";

        process = Process.Start(startInfo) ?? throw new InvalidOperationException("SleepySource could not start its compatibility engine.");

        var deadline = DateTime.UtcNow.AddSeconds(12);
        while (DateTime.UtcNow < deadline)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (process.HasExited)
                throw new InvalidOperationException($"The SleepySource compatibility engine exited during startup (code {process.ExitCode}).");
            if (await IsHealthyAsync(cancellationToken))
                return;
            await Task.Delay(120, cancellationToken);
        }

        throw new TimeoutException("SleepySource could not start the local service on 127.0.0.1:17891.");
    }

    private async Task<bool> IsHealthyAsync(CancellationToken cancellationToken)
    {
        try
        {
            using var response = await http.GetAsync(HealthUrl, cancellationToken);
            return response.IsSuccessStatusCode;
        }
        catch
        {
            return false;
        }
    }

    public void Stop()
    {
        var p = process;
        process = null;
        if (p is null)
            return;
        try
        {
            if (!p.HasExited)
            {
                p.Kill(entireProcessTree: true);
                p.WaitForExit(2500);
            }
        }
        catch { }
        finally
        {
            p.Dispose();
        }
    }

    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        Stop();
        http.Dispose();
    }
}
