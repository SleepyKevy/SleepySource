using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using System.Diagnostics;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Text.Json;

namespace SleepyMusic;

internal sealed class MainForm : Form
{
    private const string AppTitle = "SleepyMusic 1.0.0";
    private const int WM_CLOSE = 0x0010, WM_NCHITTEST = 0x0084, WM_NCLBUTTONDOWN = 0x00A1, WM_APPCOMMAND = 0x0319;
    private const int HTCAPTION = 2, HTLEFT = 10, HTRIGHT = 11, HTTOP = 12, HTTOPLEFT = 13, HTTOPRIGHT = 14, HTBOTTOM = 15, HTBOTTOMLEFT = 16, HTBOTTOMRIGHT = 17;
    private const int ResizeBorder = 7, WS_THICKFRAME = 0x00040000, WS_SYSMENU = 0x00080000, WS_MINIMIZEBOX = 0x00020000, WS_MAXIMIZEBOX = 0x00010000;
    private const int DWMWA_USE_IMMERSIVE_DARK_MODE = 20, DWMWA_WINDOW_CORNER_PREFERENCE = 33, DWMWA_BORDER_COLOR = 34, DWMWCP_ROUND = 2;
    private readonly WebView2 webView;
    private readonly BackendHost backend;
    private readonly AudioEngine audioEngine = new();
    private readonly System.Windows.Forms.Timer audioStateTimer;
    private readonly Icon applicationIcon;
    private readonly NotifyIcon trayIcon;
    private readonly ContextMenuStrip trayMenu;
    private readonly Panel titleBar;
    private readonly Button maximizeButton;
    private bool shutdownStarted, miniMode;
    private Rectangle normalBounds;

    [DllImport("dwmapi.dll")] private static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);
    [DllImport("user32.dll")] private static extern bool ReleaseCapture();
    [DllImport("user32.dll")] private static extern IntPtr SendMessage(IntPtr hWnd, int msg, IntPtr wParam, IntPtr lParam);

    public MainForm()
    {
        Text = AppTitle; StartPosition = FormStartPosition.CenterScreen; Size = new Size(1280, 820); MinimumSize = new Size(980, 680);
        BackColor = Color.FromArgb(6, 8, 13); FormBorderStyle = FormBorderStyle.None; KeyPreview = true; DoubleBuffered = true;
        applicationIcon = LoadApplicationIcon(); Icon = applicationIcon;
        backend = new BackendHost(PickFolderAsync, PickFilesAsync);
        audioEngine.TrackEnded += () => BeginInvoke(async () => await ExecuteUiScriptAsync("window.sleepyNativePlayerEnded && window.sleepyNativePlayerEnded();"));
        audioEngine.PlaybackError += message => BeginInvoke(async () => await SendPlayerErrorAsync(message));
        audioStateTimer = new System.Windows.Forms.Timer { Interval = 250 };
        audioStateTimer.Tick += async (_,_) => await PushPlayerStateAsync();

        titleBar = new Panel { Dock=DockStyle.Top, Height=30, BackColor=Color.FromArgb(6,8,13), Padding=new Padding(9,0,0,0) };
        var titleIcon = new PictureBox { Dock=DockStyle.Left, Width=23, SizeMode=PictureBoxSizeMode.CenterImage, Image=new Bitmap(applicationIcon.ToBitmap(),16,16), BackColor=Color.Transparent };
        var titleLabel = new Label { Dock=DockStyle.Fill, Text=AppTitle, TextAlign=ContentAlignment.MiddleLeft, ForeColor=Color.FromArgb(238,238,245), Font=new Font("Segoe UI",9F), BackColor=Color.Transparent };
        var closeButton=Caption("×", (_,_)=>BeginExit(), true); maximizeButton=Caption("□", (_,_)=>ToggleMaximize()); var minButton=Caption("—", (_,_)=>WindowState=FormWindowState.Minimized);
        var buttons=new FlowLayoutPanel { Dock=DockStyle.Right, Width=138, Height=29, FlowDirection=FlowDirection.LeftToRight, WrapContents=false, Margin=Padding.Empty, Padding=Padding.Empty, BackColor=Color.FromArgb(6,8,13) };
        buttons.Controls.Add(minButton); buttons.Controls.Add(maximizeButton); buttons.Controls.Add(closeButton);
        var divider=new Panel { Dock=DockStyle.Bottom, Height=1, BackColor=Color.FromArgb(32,48,71) };
        titleBar.Controls.Add(titleLabel); titleBar.Controls.Add(titleIcon); titleBar.Controls.Add(buttons); titleBar.Controls.Add(divider); divider.BringToFront();
        AttachDrag(titleBar); AttachDrag(titleLabel); AttachDrag(titleIcon);

        webView=new WebView2 { Dock=DockStyle.Fill, DefaultBackgroundColor=Color.FromArgb(6,8,13), AllowExternalDrop=false, TabStop=true };
        Controls.Add(webView); Controls.Add(titleBar);

        trayMenu=new ContextMenuStrip();
        trayMenu.Items.Add("Open SleepyMusic", null, (_,_)=>Restore());
        trayMenu.Items.Add("Play / Pause", null, (_,_)=>SendPlayerCommand("playpause"));
        trayMenu.Items.Add("Previous", null, (_,_)=>SendPlayerCommand("previous"));
        trayMenu.Items.Add("Next", null, (_,_)=>SendPlayerCommand("next"));
        trayMenu.Items.Add(new ToolStripSeparator());
        trayMenu.Items.Add("Open SleepyMusic_Data", null, (_,_)=>OpenData());
        trayMenu.Items.Add("Exit SleepyMusic", null, (_,_)=>BeginExit());
        trayIcon=new NotifyIcon { Text="SleepyMusic 1.0.0 — Made by SleepyKev • 2026", Icon=applicationIcon, ContextMenuStrip=trayMenu, Visible=true };
        trayIcon.DoubleClick += (_,_)=>Restore();

        Shown += OnShown; FormClosing += OnClosing; Resize += (_,_)=>maximizeButton.Text=WindowState==FormWindowState.Maximized?"❐":"□";
    }

    private static Button Caption(string text, EventHandler click, bool close=false)
    {
        var b=new Button { Width=46, Height=29, Text=text, FlatStyle=FlatStyle.Flat, TabStop=false, ForeColor=Color.FromArgb(237,237,237), BackColor=Color.FromArgb(6,8,13), Font=new Font("Segoe UI", text=="—"?10F:11F), Margin=Padding.Empty, Padding=Padding.Empty };
        b.FlatAppearance.BorderSize=0; b.Click+=click; b.MouseEnter+=(_,_)=>b.BackColor=close?Color.FromArgb(196,43,35):Color.FromArgb(18,36,58); b.MouseLeave+=(_,_)=>b.BackColor=Color.FromArgb(6,8,13); return b;
    }
    private void AttachDrag(Control c) => c.MouseDown += (_,e)=>{ if(e.Button!=MouseButtons.Left)return; if(e.Clicks>=2){ToggleMaximize();return;} ReleaseCapture(); SendMessage(Handle,WM_NCLBUTTONDOWN,(IntPtr)HTCAPTION,IntPtr.Zero); };
    private static Icon LoadApplicationIcon(){ try{var p=Path.Combine(AppContext.BaseDirectory,"assets","app.ico"); if(File.Exists(p))return new Icon(p);}catch{} try{using var i=Icon.ExtractAssociatedIcon(Environment.ProcessPath!); if(i!=null)return (Icon)i.Clone();}catch{} return (Icon)SystemIcons.Application.Clone(); }

    protected override CreateParams CreateParams { get { var p=base.CreateParams; p.Style|=WS_THICKFRAME|WS_SYSMENU|WS_MINIMIZEBOX|WS_MAXIMIZEBOX; return p; } }
    protected override void OnHandleCreated(EventArgs e){base.OnHandleCreated(e); try{var s=sizeof(int);var dark=1;DwmSetWindowAttribute(Handle,DWMWA_USE_IMMERSIVE_DARK_MODE,ref dark,s); if(OperatingSystem.IsWindowsVersionAtLeast(10,0,22000)){var c=DWMWCP_ROUND;DwmSetWindowAttribute(Handle,DWMWA_WINDOW_CORNER_PREFERENCE,ref c,s);var none=unchecked((int)0xFFFFFFFE);DwmSetWindowAttribute(Handle,DWMWA_BORDER_COLOR,ref none,s);}}catch{} }

    protected override void WndProc(ref Message m)
    {
        if(m.Msg==WM_CLOSE){BeginExit();return;}
        if(m.Msg==WM_APPCOMMAND){var cmd=(int)((m.LParam.ToInt64()>>16)&0x0fff); if(cmd==11){SendPlayerCommand("next");return;} if(cmd==12){SendPlayerCommand("previous");return;} if(cmd==13){SendPlayerCommand("stop");return;} if(cmd==14){SendPlayerCommand("playpause");return;}}
        if(m.Msg==WM_NCHITTEST && !miniMode && WindowState==FormWindowState.Normal){base.WndProc(ref m); if((int)m.Result==1){var lp=m.LParam.ToInt64();var sp=new Point(unchecked((short)(lp&0xffff)),unchecked((short)((lp>>16)&0xffff)));var p=PointToClient(sp);var l=p.X<=ResizeBorder;var r=p.X>=ClientSize.Width-ResizeBorder;var t=p.Y<=ResizeBorder;var b=p.Y>=ClientSize.Height-ResizeBorder;if(t&&l)m.Result=(IntPtr)HTTOPLEFT;else if(t&&r)m.Result=(IntPtr)HTTOPRIGHT;else if(b&&l)m.Result=(IntPtr)HTBOTTOMLEFT;else if(b&&r)m.Result=(IntPtr)HTBOTTOMRIGHT;else if(l)m.Result=(IntPtr)HTLEFT;else if(r)m.Result=(IntPtr)HTRIGHT;else if(t)m.Result=(IntPtr)HTTOP;else if(b)m.Result=(IntPtr)HTBOTTOM;} return;}
        base.WndProc(ref m);
    }

    private async void OnShown(object? s, EventArgs e)
    {
        Shown-=OnShown;
        try{using var cts=new CancellationTokenSource(TimeSpan.FromSeconds(20));await backend.StartAsync(cts.Token);await InitWebView();}
        catch(Exception ex){MessageBox.Show(this,"SleepyMusic 1.0.0 could not start.\n\n"+ex.Message,AppTitle,MessageBoxButtons.OK,MessageBoxIcon.Error);BeginExit();}
    }
    private async Task InitWebView()
    {
        Directory.CreateDirectory(AppUtil.RuntimeDataDir);var env=await CoreWebView2Environment.CreateAsync(null,AppUtil.RuntimeDataDir);await webView.EnsureCoreWebView2Async(env);var c=webView.CoreWebView2;
        c.Settings.IsScriptEnabled=true;c.Settings.AreDefaultContextMenusEnabled=false;c.Settings.AreDevToolsEnabled=false;c.Settings.IsStatusBarEnabled=false;c.Settings.IsZoomControlEnabled=false;c.Settings.AreBrowserAcceleratorKeysEnabled=false;
        c.NavigationStarting+=(s,e)=>{if(e.Uri.StartsWith(BackendHost.BaseUrl,StringComparison.OrdinalIgnoreCase)||e.Uri.StartsWith("http://localhost:17894/",StringComparison.OrdinalIgnoreCase))return;e.Cancel=true;try{Process.Start(new ProcessStartInfo(e.Uri){UseShellExecute=true});}catch{}};
        c.NewWindowRequested+=(s,e)=>{e.Handled=true;try{Process.Start(new ProcessStartInfo(e.Uri){UseShellExecute=true});}catch{}};
        c.WebMessageReceived += OnWebMessage;
        c.Navigate(BackendHost.BaseUrl);
        audioStateTimer.Start();
    }
    private async void OnWebMessage(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        try
        {
            using var doc=JsonDocument.Parse(e.WebMessageAsJson);
            if(!doc.RootElement.TryGetProperty("type",out var t))return;
            var type=t.GetString();
            var requestId=doc.RootElement.TryGetProperty("requestId",out var r)?r.GetString():null;
            if(type=="mini")SetMini(true);
            else if(type=="full")SetMini(false);
            else if(type=="open-data")OpenData();
            else if(type=="pick-folder")
            {
                var folder=await PickFolderAsync();
                if(string.IsNullOrWhiteSpace(folder)){await SendNativeResultAsync(requestId,true,null,true);return;}
                var snapshot=await backend.AddFolderFromHostAsync(folder);
                await SendNativeResultAsync(requestId,true,snapshot,false);
            }
            else if(type=="pick-files")
            {
                var files=await PickFilesAsync();
                if(files.Length==0){await SendNativeResultAsync(requestId,true,null,true);return;}
                var snapshot=await backend.AddFilesFromHostAsync(files);
                await SendNativeResultAsync(requestId,true,snapshot,false);
            }
            else if(type=="player-load")
            {
                var id=doc.RootElement.TryGetProperty("trackId",out var idEl)?idEl.GetString():null;
                var autoplay=doc.RootElement.TryGetProperty("autoplay",out var autoEl)&&autoEl.ValueKind==JsonValueKind.True;
                var position=doc.RootElement.TryGetProperty("position",out var posEl)&&posEl.TryGetDouble(out var pos)?pos:0;
                var track=string.IsNullOrWhiteSpace(id)?null:backend.GetTrack(id);
                if(track is null||!File.Exists(track.Path)){await SendPlayerErrorAsync("The selected audio file could not be found.");return;}
                try{audioEngine.Load(track.Id,track.Path,autoplay,position);await PushPlayerStateAsync();}
                catch(Exception ex){await SendPlayerErrorAsync("Native playback could not open this file: "+ex.Message);}
            }
            else if(type=="player-play"){audioEngine.Play();await PushPlayerStateAsync();}
            else if(type=="player-pause"){audioEngine.Pause();await PushPlayerStateAsync();}
            else if(type=="player-stop"){audioEngine.Stop();await PushPlayerStateAsync();}
            else if(type=="player-seek")
            {
                var seconds=doc.RootElement.TryGetProperty("seconds",out var secEl)&&secEl.TryGetDouble(out var sec)?sec:0;
                audioEngine.Seek(seconds);await PushPlayerStateAsync();
            }
            else if(type=="player-volume")
            {
                var value=doc.RootElement.TryGetProperty("value",out var volEl)&&volEl.TryGetDouble(out var vol)?vol:0.8;
                audioEngine.SetVolume(value);await PushPlayerStateAsync();
            }
        }
        catch(Exception ex)
        {
            try
            {
                using var doc=JsonDocument.Parse(e.WebMessageAsJson);
                var requestId=doc.RootElement.TryGetProperty("requestId",out var r)?r.GetString():null;
                await SendNativeResultAsync(requestId,false,ex.Message,false);
            }
            catch{}
        }
    }
    private async Task SendNativeResultAsync(string? requestId,bool ok,object? payload,bool cancelled)
    {
        if(string.IsNullOrWhiteSpace(requestId)||webView.CoreWebView2 is null)return;
        var envelope = new Dictionary<string, object?>
        {
            ["requestId"] = requestId,
            ["ok"] = ok,
            ["cancelled"] = cancelled,
            ["payload"] = payload
        };
        var json=JsonSerializer.Serialize(envelope,AppUtil.Json);
        await webView.CoreWebView2.ExecuteScriptAsync($"window.sleepyNativeResult && window.sleepyNativeResult({json})");
    }
    private async Task ExecuteUiScriptAsync(string script)
    {
        if (webView.CoreWebView2 is null || shutdownStarted) return;
        try { await webView.CoreWebView2.ExecuteScriptAsync(script); } catch { }
    }

    private async Task PushPlayerStateAsync()
    {
        if (webView.CoreWebView2 is null || shutdownStarted) return;
        var state = new Dictionary<string, object?>
        {
            ["trackId"] = audioEngine.TrackId,
            ["loaded"] = audioEngine.IsLoaded,
            ["playing"] = audioEngine.IsPlaying,
            ["position"] = audioEngine.PositionSeconds,
            ["duration"] = audioEngine.DurationSeconds,
            ["volume"] = audioEngine.Volume
        };
        var json = JsonSerializer.Serialize(state);
        await ExecuteUiScriptAsync($"window.sleepyNativePlayerState && window.sleepyNativePlayerState({json});");
    }

    private async Task SendPlayerErrorAsync(string message)
    {
        var json = JsonSerializer.Serialize(message);
        await ExecuteUiScriptAsync($"window.sleepyNativePlayerError && window.sleepyNativePlayerError({json});");
    }

    private void SetMini(bool mini)
    {
        if(mini==miniMode)return;miniMode=mini;
        if(mini){normalBounds=WindowState==FormWindowState.Normal?Bounds:RestoreBounds;WindowState=FormWindowState.Normal;MinimumSize=new Size(520,150);MaximumSize=new Size(900,220);Size=new Size(680,170);}
        else{MinimumSize=new Size(980,680);MaximumSize=Size.Empty;if(!normalBounds.IsEmpty)Bounds=normalBounds;}
    }
    private void ToggleMaximize(){if(miniMode)return;WindowState=WindowState==FormWindowState.Maximized?FormWindowState.Normal:FormWindowState.Maximized;}
    private async void SendPlayerCommand(string command){try{if(webView.CoreWebView2!=null)await webView.ExecuteScriptAsync($"window.sleepyMediaKey && window.sleepyMediaKey('{command}')");}catch{}}
    private Task<string?> PickFolderAsync() => UiPick(() => { using var d=new FolderBrowserDialog{Description="Choose a music folder",UseDescriptionForTitle=true,ShowNewFolderButton=false};return d.ShowDialog(this)==DialogResult.OK?d.SelectedPath:null; });
    private Task<string[]> PickFilesAsync() => UiPick(() => { using var d=new OpenFileDialog{Title="Add music files",Filter="Audio files|*.mp3;*.flac;*.wav;*.m4a;*.aac;*.ogg;*.opus|All files|*.*",Multiselect=true,CheckFileExists=true};return d.ShowDialog(this)==DialogResult.OK?d.FileNames:[]; });
    private Task<T> UiPick<T>(Func<T> pick){var tcs=new TaskCompletionSource<T>();BeginInvoke(()=>{try{tcs.SetResult(pick());}catch(Exception ex){tcs.SetException(ex);}});return tcs.Task;}
    private void OpenData(){Directory.CreateDirectory(AppUtil.DataDir);try{Process.Start(new ProcessStartInfo("explorer.exe",AppUtil.DataDir){UseShellExecute=true});}catch{}}
    private void Restore(){if(shutdownStarted)return;if(!Visible)Show();if(WindowState==FormWindowState.Minimized)WindowState=FormWindowState.Normal;Activate();BringToFront();}
    private void OnClosing(object? s, FormClosingEventArgs e){if(shutdownStarted)return;e.Cancel=true;BeginExit();}
    private void BeginExit(){if(shutdownStarted)return;shutdownStarted=true;try{audioStateTimer.Stop();audioEngine.Dispose();}catch{}try{trayIcon.Visible=false;}catch{} try{Hide();}catch{} _=Task.Run(()=>{try{backend.Stop();}catch{} Environment.Exit(0);});}
    protected override void Dispose(bool disposing){if(disposing){try{trayIcon.Visible=false;}catch{}audioStateTimer.Dispose();audioEngine.Dispose();trayIcon.Dispose();trayMenu.Dispose();backend.Dispose();applicationIcon.Dispose();}base.Dispose(disposing);}
}
