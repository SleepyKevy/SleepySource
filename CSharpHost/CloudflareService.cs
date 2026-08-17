using System.Diagnostics;
using System.Security.Cryptography;
using System.Text.RegularExpressions;

namespace SleepySource;

internal sealed class CloudflareService : IDisposable
{
    private const string Version="2026.7.3";
    private const string DownloadURL="https://github.com/cloudflare/cloudflared/releases/download/2026.7.3/cloudflared-windows-amd64.exe";
    private const string ExpectedSHA="8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841";
    private const long MaxBytes=100L<<20;
    private static readonly Regex UrlRx=new("https://[a-z0-9-]+\\.trycloudflare\\.com",RegexOptions.IgnoreCase|RegexOptions.Compiled);
    private readonly object gate=new();
    private readonly HttpClient http=new(){Timeout=TimeSpan.FromMinutes(3)};
    private Process? process; private bool running; private long startedAt; private string lastError="",publicURL="",runtime="";

    public CloudflareService(){var p=RuntimePath();if(ValidRuntime(p))runtime=p;}
    public object State(){lock(gate){var ready=File.Exists(runtime)&&new FileInfo(runtime).Length is >0 and <=MaxBytes;return ResponsePayloads.Cloudflare(running,startedAt,lastError,publicURL,runtime,ready,Version);}}
    public string PublicURL{get{lock(gate)return publicURL;}}
    public string WebhookURL{get{lock(gate)return publicURL.Length>0?publicURL.TrimEnd('/')+"/api/chat/kick-webhook":"";}}
    public (bool Running,string PublicURL,string LastError,bool RuntimeReady,string RuntimeVersion) HealthSnapshot(){lock(gate){var ready=File.Exists(runtime)&&new FileInfo(runtime).Length is >0 and <=MaxBytes;return(running,publicURL,lastError,ready,Version);}}

    public async Task StartAsync(CancellationToken ct)
    {
        lock(gate){if(running&&publicURL.Length>0)return;}
        Stop(); var bin=ValidRuntime(RuntimePath())?RuntimePath():await EnsureRuntimeAsync(ct);var home=Path.Combine(RuntimeDir(),"quick-home");Directory.CreateDirectory(home);
        var psi=new ProcessStartInfo{FileName=bin,UseShellExecute=false,CreateNoWindow=true,RedirectStandardOutput=true,RedirectStandardError=true,WorkingDirectory=AppContext.BaseDirectory};psi.ArgumentList.Add("tunnel");psi.ArgumentList.Add("--no-autoupdate");psi.ArgumentList.Add("--url");psi.ArgumentList.Add("http://127.0.0.1:17891");psi.Environment["HOME"]=home;psi.Environment["USERPROFILE"]=home;
        var p=new Process{StartInfo=psi,EnableRaisingEvents=true};if(!p.Start())throw new InvalidOperationException("could not start secure relay");lock(gate){process=p;running=true;startedAt=AppUtil.NowMS();lastError=publicURL="";runtime=bin;}
        var tcs=new TaskCompletionSource<string>(TaskCreationOptions.RunContinuationsAsynchronously);void line(string? text){if(text==null)return;var m=UrlRx.Match(text.Trim());if(m.Success)tcs.TrySetResult(m.Value.ToLowerInvariant());}
        p.OutputDataReceived+=(_,e)=>line(e.Data);p.ErrorDataReceived+=(_,e)=>line(e.Data);p.Exited+=(_,_)=>tcs.TrySetException(new InvalidOperationException("secure relay stopped before Cloudflare assigned a public URL"));p.BeginOutputReadLine();p.BeginErrorReadLine();
        try{var url=await tcs.Task.WaitAsync(TimeSpan.FromSeconds(35),ct);lock(gate){if(process==p){publicURL=url;lastError="";}}}
        catch(Exception ex){try{if(!p.HasExited)p.Kill(true);}catch{}lock(gate){running=false;process=null;publicURL="";startedAt=0;lastError=ex is TimeoutException?"secure relay timed out while waiting for a temporary public URL":ex.Message;}throw new InvalidOperationException(lastError,ex);}
    }
    public void Stop(){Process? p;lock(gate){p=process;process=null;running=false;publicURL="";startedAt=0;}try{if(p!=null&&!p.HasExited)p.Kill(true);}catch{}try{p?.Dispose();}catch{}}

    private async Task<string> EnsureRuntimeAsync(CancellationToken ct){var dest=RuntimePath();Directory.CreateDirectory(Path.GetDirectoryName(dest)!);var tmp=dest+".download";try{File.Delete(tmp);}catch{}using var resp=await http.GetAsync(DownloadURL,HttpCompletionOption.ResponseHeadersRead,ct);if(!resp.IsSuccessStatusCode)throw new InvalidOperationException($"secure relay runtime download returned HTTP {(int)resp.StatusCode}");await using(var input=await resp.Content.ReadAsStreamAsync(ct))await using(var output=new FileStream(tmp,FileMode.Create,FileAccess.Write,FileShare.None)){var buffer=new byte[128*1024];long total=0;using var sha=SHA256.Create();while(true){var n=await input.ReadAsync(buffer,ct);if(n<=0)break;total+=n;if(total>MaxBytes)throw new InvalidOperationException("secure relay runtime download is too large");await output.WriteAsync(buffer.AsMemory(0,n),ct);sha.TransformBlock(buffer,0,n,null,0);}sha.TransformFinalBlock([],0,0);var sum=Convert.ToHexString(sha.Hash!).ToLowerInvariant();if(total<=0||!sum.Equals(ExpectedSHA,StringComparison.OrdinalIgnoreCase))throw new InvalidOperationException($"secure relay runtime verification failed; expected the official Cloudflare {Version} Windows x64 build");}File.Move(tmp,dest,true);return dest;}
    private static string RuntimeDir(){var baseDir=Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);if(string.IsNullOrWhiteSpace(baseDir))baseDir=Path.GetTempPath();return Path.Combine(baseDir,"SleepySource","runtime");}
    private static string RuntimePath()=>Path.Combine(RuntimeDir(),$"cloudflared-{Version}-windows-amd64.exe");
    private static bool ValidRuntime(string path){try{if(!File.Exists(path))return false;var fi=new FileInfo(path);if(fi.Length<=0||fi.Length>MaxBytes)return false;using var s=File.OpenRead(path);return Convert.ToHexString(SHA256.HashData(s)).Equals(ExpectedSHA,StringComparison.OrdinalIgnoreCase);}catch{return false;}}
    public void Dispose(){Stop();http.Dispose();}
}
