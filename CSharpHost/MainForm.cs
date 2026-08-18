using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using System.Diagnostics;
using System.Drawing;
using System.Runtime.InteropServices;

namespace SleepySource;

internal sealed class MainForm : Form
{
    private const string AppTitle = "SleepySource 1.0.0";
    private const string LocalHost = "https://sleepysource.local/";
    private const string EngineHost = "http://127.0.0.1:17891/";
    private const int WM_CLOSE = 0x0010;
    private const int GracefulExitBudgetMs = 500;
    private const int DWMWA_USE_IMMERSIVE_DARK_MODE = 20;
    private const int DWMWA_BORDER_COLOR = 34;
    private const int DWMWA_CAPTION_COLOR = 35;
    private const int DWMWA_TEXT_COLOR = 36;

    private readonly WebView2 webView;
    private readonly BackendHost backend = new();
    private readonly Icon applicationIcon;
    private readonly NotifyIcon trayIcon;
    private readonly ContextMenuStrip trayMenu;
    private bool backendReady;
    private bool dashboardOpen;
    private bool shutdownStarted;

    [DllImport("dwmapi.dll")]
    private static extern int DwmSetWindowAttribute(
        IntPtr hwnd,
        int dwAttribute,
        ref int pvAttribute,
        int cbAttribute);

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

        // Standard Windows notification-area (system tray) icon. SleepySource does
        // not use tray balloon notifications. Windows decides whether an app icon is
        // pinned directly to the tray or placed in the hidden-icons overflow area.
        trayIcon = new NotifyIcon
        {
            Text = "SleepySource 1.0.0 — Made by SleepyKev • 2026",
            Icon = applicationIcon,
            ContextMenuStrip = trayMenu,
            Visible = true
        };
        trayIcon.DoubleClick += (_, _) => RestoreFromTray();

        Shown += OnShown;
        FormClosing += OnFormClosing;
        Resize += (_, _) =>
        {
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

    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        ApplyWindowsTitleBarTheme();
    }

    private void ApplyWindowsTitleBarTheme()
    {
        // Direct caption/border color attributes are supported on Windows 11.
        // Windows 10 simply keeps its normal system-managed title bar.
        if (!OperatingSystem.IsWindowsVersionAtLeast(10, 0, 22000))
            return;

        try
        {
            var darkMode = 1;
            var captionColor = ToColorRef(Color.FromArgb(2, 5, 10));
            var textColor = ToColorRef(Color.White);
            var borderColor = captionColor;
            var valueSize = sizeof(int);

            _ = DwmSetWindowAttribute(Handle, DWMWA_USE_IMMERSIVE_DARK_MODE, ref darkMode, valueSize);
            _ = DwmSetWindowAttribute(Handle, DWMWA_CAPTION_COLOR, ref captionColor, valueSize);
            _ = DwmSetWindowAttribute(Handle, DWMWA_TEXT_COLOR, ref textColor, valueSize);
            _ = DwmSetWindowAttribute(Handle, DWMWA_BORDER_COLOR, ref borderColor, valueSize);
        }
        catch
        {
            // Keep the normal Windows title bar if DWM customization is unavailable.
        }
    }

    private static int ToColorRef(Color color) =>
        color.R | (color.G << 8) | (color.B << 16);

    protected override void WndProc(ref Message m)
    {
        if (m.Msg == WM_CLOSE)
        {
            BeginExit();
            return;
        }

        base.WndProc(ref m);
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
                "SleepySource 1.0.0 could not start.\n\n" + ex.Message,
                AppTitle,
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
            BeginExit();
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

        if (string.Equals(message, "splash-stage-complete", StringComparison.Ordinal))
        {
            // The visual loader intentionally pauses near completion until the real
            // in-process backend is ready. This keeps the final 100%/Ready state tied
            // to actual startup readiness instead of a purely cosmetic timer.
            if (!backendReady)
            {
                var deadline = DateTime.UtcNow.AddSeconds(10);
                while (!backendReady && DateTime.UtcNow < deadline)
                    await Task.Delay(100);
            }

            if (backendReady && !dashboardOpen)
            {
                try { webView.CoreWebView2.PostWebMessageAsString("backend-ready"); }
                catch { }
            }
            return;
        }

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

    private void ExitApplication() => BeginExit();

    private void OnFormClosing(object? sender, FormClosingEventArgs e)
    {
        e.Cancel = true;
        BeginExit();
    }

    private void BeginExit()
    {
        if (shutdownStarted)
            return;

        shutdownStarted = true;
        backendReady = false;

        // Dedicated watchdog thread: unlike a ThreadPool timer, this cannot be delayed
        // by busy backend cleanup. X-close is therefore bounded to roughly 500 ms.
        var watchdog = new Thread(() =>
        {
            Thread.Sleep(GracefulExitBudgetMs);
            ForceProcessExit();
        })
        {
            IsBackground = true,
            Name = "SleepySource Exit Watchdog"
        };
        watchdog.Start();

        // Remove visible UI immediately. No WebView2 disposal is performed on this
        // path because native WebView shutdown can stall the desktop thread.
        try { trayIcon.Visible = false; } catch { }
        try { ShowInTaskbar = false; } catch { }
        try { Hide(); } catch { }

        _ = Task.Run(() =>
        {
            try { backend.Stop(); }
            catch { }
            Environment.Exit(0);
        });
    }

    private static void ForceProcessExit() => Environment.Exit(0);

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
