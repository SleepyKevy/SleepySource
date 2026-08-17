using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using System.Diagnostics;
using System.Drawing;

namespace SleepySource;

internal sealed class MainForm : Form
{
    private const string AppTitle = "SleepySource 1.0";
    private const string LocalHost = "https://sleepysource.local/";
    private const string EngineHost = "http://127.0.0.1:17891/";
    private static readonly TimeSpan GracefulExitBudget = TimeSpan.FromMilliseconds(1500);

    private readonly WebView2 webView;
    private readonly BackendHost backend = new();
    private readonly Icon applicationIcon;
    private readonly NotifyIcon trayIcon;
    private readonly ContextMenuStrip trayMenu;
    private bool backendReady;
    private bool dashboardOpen;
    private bool shutdownStarted;
    private bool shutdownComplete;

    public MainForm()
    {
        Text = AppTitle;
        StartPosition = FormStartPosition.CenterScreen;
        Size = new Size(1450, 920);
        MinimumSize = new Size(980, 680);
        BackColor = Color.FromArgb(2, 5, 10);
        KeyPreview = true;

        applicationIcon = LoadApplicationIcon();
        Icon = applicationIcon;

        webView = new WebView2
        {
            Dock = DockStyle.Fill,
            DefaultBackgroundColor = Color.FromArgb(2, 5, 10),
            AllowExternalDrop = false,
            TabStop = true
        };
        Controls.Add(webView);

        trayMenu = new ContextMenuStrip();
        trayMenu.Items.Add("Open SleepySource", null, (_, _) => RestoreFromTray());
        trayMenu.Items.Add("Open SleepySource_Data", null, (_, _) => OpenDataFolder());
        trayMenu.Items.Add(new ToolStripSeparator());
        trayMenu.Items.Add("Exit SleepySource", null, (_, _) => ExitApplication());

        // This is a standard Windows notification-area (system tray) icon.
        // SleepySource never displays balloon notifications from this icon.
        trayIcon = new NotifyIcon
        {
            Text = "SleepySource 1.0 — Made by SleepyKev • 2026",
            Icon = applicationIcon,
            ContextMenuStrip = trayMenu,
            Visible = true
        };
        trayIcon.DoubleClick += (_, _) => RestoreFromTray();

        Shown += OnShown;
        FormClosing += OnFormClosing;
        Resize += (_, _) =>
        {
            // Keep the window represented on the Windows taskbar when minimized.
            // The notification-area icon remains available at the same time.
            if (WindowState == FormWindowState.Minimized)
                ShowInTaskbar = true;
        };
    }

    private static Icon LoadApplicationIcon()
    {
        var iconPath = Path.Combine(AppContext.BaseDirectory, "assets", "app.ico");
        try
        {
            if (File.Exists(iconPath))
                return new Icon(iconPath);
        }
        catch
        {
        }

        try
        {
            var executablePath = Environment.ProcessPath;
            if (!string.IsNullOrWhiteSpace(executablePath))
            {
                using var embedded = Icon.ExtractAssociatedIcon(executablePath);
                if (embedded is not null)
                    return (Icon)embedded.Clone();
            }
        }
        catch
        {
        }

        return (Icon)SystemIcons.Application.Clone();
    }

    private async void OnShown(object? sender, EventArgs e)
    {
        Shown -= OnShown;
        trayIcon.Visible = true;
        try
        {
            using var startupCts = new CancellationTokenSource(TimeSpan.FromSeconds(16));
            var backendTask = backend.StartAsync(startupCts.Token);
            await InitializeWebViewAsync();
            await backendTask;
            backendReady = true;
        }
        catch (Exception ex)
        {
            MessageBox.Show(
                this,
                "SleepySource 1.0 could not start.\n\n" + ex.Message,
                AppTitle,
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
            Close();
        }
    }

    private async Task InitializeWebViewAsync()
    {
        var userDataFolder = Path.Combine(
            AppContext.BaseDirectory,
            "SleepySource_Data",
            "DesktopRuntime-CSharp");
        Directory.CreateDirectory(userDataFolder);

        var environment = await CoreWebView2Environment.CreateAsync(
            browserExecutableFolder: null,
            userDataFolder: userDataFolder);
        await webView.EnsureCoreWebView2Async(environment);

        var core = webView.CoreWebView2;
        core.Settings.AreDefaultContextMenusEnabled = false;
        core.Settings.AreDevToolsEnabled = false;
        core.Settings.IsStatusBarEnabled = false;
        core.Settings.IsZoomControlEnabled = false;
        core.Settings.AreBrowserAcceleratorKeysEnabled = false;

        core.SetVirtualHostNameToFolderMapping(
            "sleepysource.local",
            AppContext.BaseDirectory,
            CoreWebView2HostResourceAccessKind.Allow);

        core.WebMessageReceived += OnWebMessageReceived;
        core.NavigationStarting += OnNavigationStarting;
        core.NavigationCompleted += OnNavigationCompleted;
        core.NewWindowRequested += OnNewWindowRequested;
        core.Navigate(LocalHost + "web/enter.html");
    }

    private async void OnWebMessageReceived(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        string? message;
        try { message = e.TryGetWebMessageAsString(); }
        catch { return; }

        if (!string.Equals(message, "open", StringComparison.Ordinal) || dashboardOpen)
            return;

        if (!backendReady)
        {
            var deadline = DateTime.UtcNow.AddSeconds(10);
            while (!backendReady && DateTime.UtcNow < deadline)
                await Task.Delay(100);
        }
        if (!backendReady)
            return;

        dashboardOpen = true;
        webView.CoreWebView2.Navigate(EngineHost + "designer");
    }

    private void OnNavigationStarting(object? sender, CoreWebView2NavigationStartingEventArgs e)
    {
        if (e.NavigationKind == CoreWebView2NavigationKind.BackOrForward)
        {
            e.Cancel = true;
            return;
        }

        if (e.Uri.StartsWith(LocalHost, StringComparison.OrdinalIgnoreCase) ||
            e.Uri.StartsWith(EngineHost, StringComparison.OrdinalIgnoreCase) ||
            e.Uri.StartsWith("http://localhost:17891/", StringComparison.OrdinalIgnoreCase))
            return;

        e.Cancel = true;
        OpenExternal(e.Uri);
    }

    private async void OnNavigationCompleted(object? sender, CoreWebView2NavigationCompletedEventArgs e)
    {
        if (!e.IsSuccess || !dashboardOpen)
            return;

        var source = webView.Source?.AbsoluteUri ?? string.Empty;
        if (!source.StartsWith(EngineHost, StringComparison.OrdinalIgnoreCase) &&
            !source.StartsWith("http://localhost:17891/", StringComparison.OrdinalIgnoreCase))
            return;

        try
        {
            await webView.CoreWebView2.CallDevToolsProtocolMethodAsync(
                "Page.resetNavigationHistory",
                "{}");
        }
        catch
        {
        }
    }

    private void OnNewWindowRequested(object? sender, CoreWebView2NewWindowRequestedEventArgs e)
    {
        e.Handled = true;
        OpenExternal(e.Uri);
    }

    private static void OpenExternal(string uri)
    {
        try
        {
            Process.Start(new ProcessStartInfo(uri) { UseShellExecute = true });
        }
        catch { }
    }

    private void OpenDataFolder()
    {
        var path = Path.Combine(AppContext.BaseDirectory, "SleepySource_Data");
        Directory.CreateDirectory(path);
        try { Process.Start(new ProcessStartInfo("explorer.exe", path) { UseShellExecute = true }); }
        catch { }
    }

    private void RestoreFromTray()
    {
        if (shutdownStarted)
            return;

        ShowInTaskbar = true;
        Show();
        WindowState = FormWindowState.Normal;
        Activate();
        BringToFront();
    }

    private void ExitApplication()
    {
        Close();
    }

    private void OnFormClosing(object? sender, FormClosingEventArgs e)
    {
        if (shutdownComplete)
            return;

        // X means exit. Do not block the WinForms UI thread while Kestrel, the
        // media watcher, Cloudflare, and WebView2 shut down.
        e.Cancel = true;
        if (shutdownStarted)
            return;

        shutdownStarted = true;
        backendReady = false;

        // Make SleepySource disappear immediately from the desktop, taskbar,
        // and notification area before doing background cleanup.
        trayIcon.Visible = false;
        ShowInTaskbar = false;
        Hide();

        try { webView.CoreWebView2?.Stop(); } catch { }
        try { webView.Dispose(); } catch { }

        _ = FinishShutdownAsync();
    }

    private async Task FinishShutdownAsync()
    {
        var gracefulStop = Task.Run(() =>
        {
            try { backend.Stop(); }
            catch { }
        });

        var completed = await Task.WhenAny(gracefulStop, Task.Delay(GracefulExitBudget));
        if (completed != gracefulStop)
        {
            // Never leave an invisible SleepySource process sitting in Task Manager.
            // The normal graceful path is given 1.5 seconds first.
            Environment.Exit(0);
            return;
        }

        shutdownComplete = true;
        if (IsDisposed || !IsHandleCreated)
        {
            Environment.Exit(0);
            return;
        }

        try
        {
            BeginInvoke(new Action(() =>
            {
                shutdownComplete = true;
                Close();
                Application.ExitThread();
            }));
        }
        catch
        {
            Environment.Exit(0);
        }
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            trayIcon.Visible = false;
            trayIcon.Dispose();
            trayMenu.Dispose();
            webView.Dispose();
            backend.Dispose();
            applicationIcon.Dispose();
        }
        base.Dispose(disposing);
    }
}
