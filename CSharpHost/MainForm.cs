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

    private readonly WebView2 webView;
    private readonly BackendHost backend = new();
    private readonly NotifyIcon trayIcon;
    private readonly ContextMenuStrip trayMenu;
    private bool backendReady;
    private bool dashboardOpen;

    public MainForm()
    {
        Text = AppTitle;
        StartPosition = FormStartPosition.CenterScreen;
        Size = new Size(1450, 920);
        MinimumSize = new Size(980, 680);
        BackColor = Color.FromArgb(2, 5, 10);
        KeyPreview = true;

        var iconPath = Path.Combine(AppContext.BaseDirectory, "assets", "app.ico");
        if (File.Exists(iconPath))
            Icon = new Icon(iconPath);

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

        trayIcon = new NotifyIcon
        {
            Text = "SleepySource 1.0 — Made by SleepyKev • 2026",
            Icon = Icon,
            ContextMenuStrip = trayMenu,
            Visible = true
        };
        trayIcon.DoubleClick += (_, _) => RestoreFromTray();

        Shown += OnShown;
        FormClosing += OnFormClosing;
        Resize += (_, _) =>
        {
            // Keep the window represented on the Windows taskbar when minimized.
            // The tray icon remains visible at the same time for quick access.
            if (WindowState == FormWindowState.Minimized)
                ShowInTaskbar = true;
        };
    }

    private async void OnShown(object? sender, EventArgs e)
    {
        Shown -= OnShown;
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
        // Closing the window with X exits SleepySource completely.
        trayIcon.Visible = false;
        backend.Stop();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            trayIcon.Dispose();
            trayMenu.Dispose();
            webView.Dispose();
            backend.Dispose();
        }
        base.Dispose(disposing);
    }
}
