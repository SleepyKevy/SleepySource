using System.Runtime.InteropServices;
using System.Threading;

namespace SleepyMusic;

internal static class Program
{
    private const string MutexName = @"Local\SleepyKev.SleepyMusic.1.0.0";
    [DllImport("shell32.dll", CharSet=CharSet.Unicode, SetLastError=false)] private static extern int SetCurrentProcessExplicitAppUserModelID(string appId);

    [STAThread]
    private static void Main(string[] args)
    {
        using var mutex=new Mutex(true,MutexName,out var ownsMutex);
        if(!ownsMutex){if(!args.Contains("--headless",StringComparer.OrdinalIgnoreCase))MessageBox.Show("SleepyMusic 1.0.0 is already running.","SleepyMusic 1.0.0",MessageBoxButtons.OK,MessageBoxIcon.Information);return;}
        if(args.Contains("--headless",StringComparer.OrdinalIgnoreCase)){using var backend=new BackendHost();backend.StartAsync(CancellationToken.None).GetAwaiter().GetResult();using var wait=new ManualResetEventSlim(false);wait.Wait();return;}
        try{_=SetCurrentProcessExplicitAppUserModelID("SleepyKev.SleepyMusic.1.0.0");}catch{}
        ApplicationConfiguration.Initialize();
        Application.Run(new MainForm());
    }
}
