using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using System.Diagnostics;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Text.Json;

namespace SleepyMusic;

internal sealed class MainForm : Form
{
    private const string AppTitle="SleepyMusic 1.0.0";
    private const int WM_CLOSE=0x0010,WM_APPCOMMAND=0x0319;
    private const int APPCOMMAND_MEDIA_NEXTTRACK=11,APPCOMMAND_MEDIA_PREVIOUSTRACK=12,APPCOMMAND_MEDIA_STOP=13,APPCOMMAND_MEDIA_PLAY_PAUSE=14;
    private const int GracefulExitBudgetMs=500,DWMWA_USE_IMMERSIVE_DARK_MODE=20,DWMWA_BORDER_COLOR=34,DWMWA_CAPTION_COLOR=35,DWMWA_TEXT_COLOR=36;
    private readonly WebView2 webView;
    private readonly BackendHost backend=new();
    private readonly Icon applicationIcon;
    private readonly NotifyIcon trayIcon;
    private readonly ContextMenuStrip trayMenu;
    private bool shutdownStarted;
    [DllImport("dwmapi.dll")] private static extern int DwmSetWindowAttribute(IntPtr hwnd,int dwAttribute,ref int pvAttribute,int cbAttribute);

    public MainForm()
    {
        Text=AppTitle;StartPosition=FormStartPosition.CenterScreen;Size=new Size(1450,920);MinimumSize=new Size(1040,700);BackColor=Color.FromArgb(6,8,13);KeyPreview=true;
        applicationIcon=LoadApplicationIcon();Icon=applicationIcon;
        webView=new WebView2{Dock=DockStyle.Fill,DefaultBackgroundColor=Color.FromArgb(6,8,13),AllowExternalDrop=false,TabStop=true};Controls.Add(webView);
        trayMenu=new ContextMenuStrip();
        trayMenu.Items.Add("Open SleepyMusic",null,(_,_)=>RestoreFromTray());trayMenu.Items.Add(new ToolStripSeparator());
        trayMenu.Items.Add("Play / Pause",null,async(_,_)=>await SendMediaCommandAsync("playpause"));trayMenu.Items.Add("Previous Track",null,async(_,_)=>await SendMediaCommandAsync("previous"));trayMenu.Items.Add("Next Track",null,async(_,_)=>await SendMediaCommandAsync("next"));
        trayMenu.Items.Add(new ToolStripSeparator());trayMenu.Items.Add("Open SleepyMusic_Data",null,(_,_)=>OpenDataFolder());trayMenu.Items.Add("Exit SleepyMusic",null,(_,_)=>BeginExit());
        trayIcon=new NotifyIcon{Text="SleepyMusic 1.0.0 — Made by SleepyKev • 2026",Icon=applicationIcon,ContextMenuStrip=trayMenu,Visible=true};trayIcon.DoubleClick+=(_,_)=>RestoreFromTray();
        Shown+=OnShown;FormClosing+=OnFormClosing;Resize+=(_,_)=>{if(WindowState==FormWindowState.Minimized)ShowInTaskbar=true;};
    }
    private static Icon LoadApplicationIcon(){var path=Path.Combine(AppContext.BaseDirectory,"assets","app.ico");try{if(File.Exists(path))return new Icon(path);}catch{}try{var exe=Environment.ProcessPath;if(!string.IsNullOrWhiteSpace(exe)){using var embedded=Icon.ExtractAssociatedIcon(exe);if(embedded is not null)return (Icon)embedded.Clone();}}catch{}return (Icon)SystemIcons.Application.Clone();}
    protected override void OnHandleCreated(EventArgs e){base.OnHandleCreated(e);ApplyWindowsTitleBarTheme();}
    private void ApplyWindowsTitleBarTheme(){if(!OperatingSystem.IsWindowsVersionAtLeast(10,0,22000))return;try{var dark=1;var caption=ToColorRef(Color.FromArgb(6,8,13));var text=ToColorRef(Color.White);var border=caption;var size=sizeof(int);_=DwmSetWindowAttribute(Handle,DWMWA_USE_IMMERSIVE_DARK_MODE,ref dark,size);_=DwmSetWindowAttribute(Handle,DWMWA_CAPTION_COLOR,ref caption,size);_=DwmSetWindowAttribute(Handle,DWMWA_TEXT_COLOR,ref text,size);_=DwmSetWindowAttribute(Handle,DWMWA_BORDER_COLOR,ref border,size);}catch{}}
    private static int ToColorRef(Color c)=>c.R|(c.G<<8)|(c.B<<16);
    protected override void WndProc(ref Message m)
    {
        if(m.Msg==WM_CLOSE){BeginExit();return;}
        if(m.Msg==WM_APPCOMMAND){var command=(int)((m.LParam.ToInt64()>>16)&0x0FFF);Task? handled=command switch{APPCOMMAND_MEDIA_NEXTTRACK=>SendMediaCommandAsync("next"),APPCOMMAND_MEDIA_PREVIOUSTRACK=>SendMediaCommandAsync("previous"),APPCOMMAND_MEDIA_PLAY_PAUSE=>SendMediaCommandAsync("playpause"),APPCOMMAND_MEDIA_STOP=>SendMediaCommandAsync("stop"),_=>null};if(handled is not null)return;}
        base.WndProc(ref m);
    }
    private async void OnShown(object? sender,EventArgs e){Shown-=OnShown;try{using var cts=new CancellationTokenSource(TimeSpan.FromSeconds(20));await backend.StartAsync(cts.Token);await InitializeWebViewAsync();}catch(Exception ex){MessageBox.Show(this,"SleepyMusic 1.0.0 could not start.\n\n"+ex.Message,AppTitle,MessageBoxButtons.OK,MessageBoxIcon.Error);BeginExit();}}
    private async Task InitializeWebViewAsync(){Directory.CreateDirectory(AppUtil.RuntimeDataDir);var env=await CoreWebView2Environment.CreateAsync(null,AppUtil.RuntimeDataDir);await webView.EnsureCoreWebView2Async(env);var core=webView.CoreWebView2;core.Settings.AreDefaultContextMenusEnabled=false;core.Settings.AreDevToolsEnabled=false;core.Settings.IsStatusBarEnabled=false;core.Settings.IsZoomControlEnabled=false;core.Settings.AreBrowserAcceleratorKeysEnabled=false;core.WebMessageReceived+=OnWebMessageReceived;core.NavigationStarting+=OnNavigationStarting;core.NewWindowRequested+=OnNewWindowRequested;core.Navigate(BackendHost.BaseUrl);}
    private async void OnWebMessageReceived(object? sender,CoreWebView2WebMessageReceivedEventArgs e){try{using var doc=JsonDocument.Parse(e.WebMessageAsJson);if(!doc.RootElement.TryGetProperty("action",out var a))return;var action=a.GetString();if(action=="addFolder")await PickAndAddFolderAsync();else if(action=="openData")OpenDataFolder();}catch{}}
    private async Task PickAndAddFolderAsync(){using var picker=new FolderBrowserDialog{Description="Choose a folder containing your local music files.",UseDescriptionForTitle=true,ShowNewFolderButton=false};if(picker.ShowDialog(this)!=DialogResult.OK||string.IsNullOrWhiteSpace(picker.SelectedPath))return;try{UseWaitCursor=true;await backend.Library.AddFolderAsync(picker.SelectedPath);await webView.CoreWebView2.ExecuteScriptAsync("window.sleepyMusicRefreshLibrary && window.sleepyMusicRefreshLibrary();");}catch(Exception ex){MessageBox.Show(this,"That folder could not be added.\n\n"+ex.Message,AppTitle,MessageBoxButtons.OK,MessageBoxIcon.Warning);}finally{UseWaitCursor=false;}}
    private void OnNavigationStarting(object? sender,CoreWebView2NavigationStartingEventArgs e){if(e.Uri.StartsWith(BackendHost.BaseUrl,StringComparison.OrdinalIgnoreCase)||e.Uri.StartsWith("http://localhost:17893/",StringComparison.OrdinalIgnoreCase))return;e.Cancel=true;OpenExternal(e.Uri);}
    private void OnNewWindowRequested(object? sender,CoreWebView2NewWindowRequestedEventArgs e){e.Handled=true;OpenExternal(e.Uri);}
    private static void OpenExternal(string uri){try{Process.Start(new ProcessStartInfo(uri){UseShellExecute=true});}catch{}}
    private async Task SendMediaCommandAsync(string command){if(webView.CoreWebView2 is null)return;try{await webView.CoreWebView2.ExecuteScriptAsync($"window.sleepyMusicMediaCommand && window.sleepyMusicMediaCommand('{command}');");}catch{}}
    private void OpenDataFolder(){Directory.CreateDirectory(AppUtil.DataDir);try{Process.Start(new ProcessStartInfo("explorer.exe",AppUtil.DataDir){UseShellExecute=true});}catch{}}
    private void RestoreFromTray(){if(shutdownStarted)return;if(!Visible)Show();if(WindowState==FormWindowState.Minimized)WindowState=FormWindowState.Normal;ShowInTaskbar=true;Activate();BringToFront();}
    private void OnFormClosing(object? sender,FormClosingEventArgs e){if(shutdownStarted)return;e.Cancel=true;BeginExit();}
    private void BeginExit(){if(shutdownStarted)return;shutdownStarted=true;var watchdog=new Thread(()=>{Thread.Sleep(GracefulExitBudgetMs);ForceProcessExit();}){IsBackground=true,Name="SleepyMusic Exit Watchdog"};watchdog.Start();try{trayIcon.Visible=false;}catch{}try{Hide();ShowInTaskbar=false;}catch{}_=Task.Run(()=>{try{backend.Stop();}catch{}Environment.Exit(0);});}
    private static void ForceProcessExit()=>Environment.Exit(0);
    protected override void Dispose(bool disposing){if(disposing){try{trayIcon.Visible=false;}catch{}trayIcon.Dispose();trayMenu.Dispose();backend.Dispose();applicationIcon.Dispose();}base.Dispose(disposing);}
}
