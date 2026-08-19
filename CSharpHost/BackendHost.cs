using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.Features;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.DependencyInjection;
using System.Net;
using System.Text;
using System.Text.Json;

namespace SleepySource;

internal sealed class BackendHost : IDisposable
{
    private WebApplication? app;
    private readonly CoreStateService core = new();
    private readonly CountdownService countdown;
    private readonly AlertService alerts;
    private readonly ChatService chat;
    private readonly SleepySourceApiService sleepyApi;
    private readonly ProfileBackupService profiles;
    private readonly MediaSessionService media;
    private readonly UpdateService updates = new();
    private readonly KickAssetsService kickAssets;
    private readonly HealthService health;

    public BackendHost()
    {
        countdown = new CountdownService(core.DataDir);
        alerts = new AlertService(core.DataDir);
        chat = new ChatService(core.DataDir);
        sleepyApi = new SleepySourceApiService(core.DataDir, chat, alerts);
        profiles = new ProfileBackupService(core);
        media = new MediaSessionService(core);
        kickAssets = new KickAssetsService(chat);
        health = new HealthService(core, chat, alerts, sleepyApi);
    }

    public async Task StartAsync(CancellationToken ct = default)
    {
        if (app is not null) return;
        await profiles.LoadDefaultAtStartupAsync();
        var options = new WebApplicationOptions { ContentRootPath = AppContext.BaseDirectory, ApplicationName = typeof(BackendHost).Assembly.GetName().Name };
        var builder = WebApplication.CreateBuilder(options);
        builder.WebHost.UseUrls("http://127.0.0.1:17891");
        builder.WebHost.ConfigureKestrel(o => o.Limits.MaxRequestBodySize = AppUtil.MaxBackupBytes + (4L << 20));
        builder.Services.Configure<FormOptions>(o => { o.MultipartBodyLengthLimit = AppUtil.MaxBackupBytes + (4L << 20); o.ValueLengthLimit = 2 << 20; o.MultipartHeadersLengthLimit = 64 << 10; });
        var web = builder.Build();
        ConfigureMiddleware(web);
        MapStatic(web);
        MapCore(web);
        MapProfiles(web);
        MapCountdown(web);
        MapAlerts(web);
        MapChat(web);
        MapKick(web);
        MapHealth(web);
        app = web;
        try { await web.StartAsync(ct); }
        catch { app = null; await web.DisposeAsync(); throw; }
        await media.StartAsync();
        sleepyApi.Start();
    }

    public void Stop()
    {
        media.Stop();
        sleepyApi.Stop();
        var running = app; app = null;
        if (running is not null) try { using var stopCts = new CancellationTokenSource(TimeSpan.FromSeconds(3)); running.StopAsync(stopCts.Token).GetAwaiter().GetResult(); } catch { }
        if (running is not null) try { running.DisposeAsync().AsTask().GetAwaiter().GetResult(); } catch { }
    }

    private void ConfigureMiddleware(WebApplication web)
    {
        web.Use(async (ctx, next) =>
        {
            var path = ctx.Request.Path.Value ?? "";
            if (!RequestAllowed(ctx.Request)) { ctx.Response.StatusCode=403; await ctx.Response.WriteAsync("local SleepySource request required"); return; }
            ctx.Response.Headers["X-Content-Type-Options"]="nosniff";
            ctx.Response.Headers["Referrer-Policy"]="no-referrer";
            ctx.Response.Headers["X-Frame-Options"]="SAMEORIGIN";
            ctx.Response.Headers["Permissions-Policy"]="camera=(), microphone=(), geolocation=(), payment=()";
            ctx.Response.Headers["Content-Security-Policy"]="default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'";
            if(path.StartsWith("/assets/",StringComparison.OrdinalIgnoreCase))ctx.Response.Headers.CacheControl="public, max-age=31536000, immutable";
            else if(path.StartsWith("/media/",StringComparison.OrdinalIgnoreCase)||path=="/font")ctx.Response.Headers.CacheControl=ctx.Request.Query.ContainsKey("v")?"private, max-age=31536000, immutable":"private, max-age=60";
            else ctx.Response.Headers.CacheControl="no-store";
            try { await next(); }
            catch(Exception ex) when (!ctx.Response.HasStarted)
            {
                var status = ex switch { InvalidDataException => 400, FileNotFoundException => 404, UnauthorizedAccessException => 401, _ => 500 };
                ctx.Response.StatusCode=status;ctx.Response.ContentType="text/plain; charset=utf-8";await ctx.Response.WriteAsync(status==500?"SleepySource request failed":ex.Message);
            }
        });
    }

    private static bool RequestAllowed(HttpRequest r)
    {
        var host=r.Host.Host.Trim('[',']').ToLowerInvariant();if(host is not("127.0.0.1" or "localhost" or "::1"))return false;
        if(HttpMethods.IsGet(r.Method)||HttpMethods.IsHead(r.Method)||HttpMethods.IsOptions(r.Method))return true;
        var origin=r.Headers.Origin.ToString().Trim();if(origin.Length==0)return true;if(origin=="null")return false;
        return Uri.TryCreate(origin,UriKind.Absolute,out var u)&&u.Scheme=="http"&&(u.Host.Equals("127.0.0.1",StringComparison.OrdinalIgnoreCase)||u.Host.Equals("localhost",StringComparison.OrdinalIgnoreCase)||u.Host=="::1")&&u.Port==17891;
    }

    private static void MapStatic(WebApplication web)
    {
        MapPage(web,"/","enter.html");MapPage(web,"/designer","index.html");MapPage(web,"/chat","chat.html");MapPage(web,"/overlay","overlay.html");MapPage(web,"/countdown","countdown.html");MapPage(web,"/alerts","alerts.html");
        web.MapMethods("/assets/{**name}",["GET","HEAD"],(HttpContext c,string name)=>FileResult(Path.Combine(AppContext.BaseDirectory,"assets",name),c));
    }
    private static void MapPage(WebApplication web,string route,string file)=>web.MapMethods(route,["GET","HEAD"],(HttpContext c)=>FileResult(Path.Combine(AppContext.BaseDirectory,"web",file),c,"text/html; charset=utf-8"));
    private static IResult FileResult(string path,HttpContext ctx,string? type=null){if(!File.Exists(path))return Results.NotFound();if(HttpMethods.IsHead(ctx.Request.Method))return Results.Ok();return Results.File(path,type??AppUtil.ContentTypeForPath(path),enableRangeProcessing:false);}

    private void MapCore(WebApplication web)
    {
        web.MapGet("/api/state",()=>J(core.Snapshot()));
        web.MapGet("/api/settings",()=>J(core.Snapshot()));
        web.MapPost("/api/settings",async (HttpRequest r,CancellationToken ct)=>{var next=await ReadJson<Settings>(r,ct)??throw new InvalidDataException("invalid settings");var current=core.SettingsSnapshot();next.CustomImage=current.CustomImage;next.CustomBackground=current.CustomBackground;await core.ApplySettingsAsync(next,ct);return J(core.Snapshot());});
        web.MapGet("/api/export-settings",()=>Results.File(JsonSerializer.SerializeToUtf8Bytes(core.SettingsSnapshot(),AppUtil.JsonIndented),"application/json","SleepySource_Settings.json"));
        web.MapPost("/api/import-settings",async (HttpRequest r,CancellationToken ct)=>{var f=await FormFile(r,"settings",2L<<20,ct);var next=JsonSerializer.Deserialize<Settings>(f.Data,AppUtil.Json)??throw new InvalidDataException("invalid settings JSON");var cur=core.SettingsSnapshot();next.CustomImage=cur.CustomImage;next.CustomBackground=cur.CustomBackground;await core.ApplySettingsAsync(next,ct);return J(core.Snapshot());});
        web.MapGet("/api/backup/export",()=>Results.File(profiles.ExportBackup(),"application/zip","SleepySource_Backup.zip"));
        web.MapPost("/api/backup/import",async (HttpRequest r,CancellationToken ct)=>{var f=await FormFile(r,"backup",AppUtil.MaxBackupBytes,ct);await profiles.RestoreBackupAsync(f.Data,chat,countdown);return J(new{ok=true,message="SleepySource backup restored"});});
        web.MapPost("/api/upload",async (HttpRequest r,CancellationToken ct)=>{var f=await FormFile(r,"image",AppUtil.MaxUploadBytes,ct,"file");var ext=ValidateMedia(f.Data,f.FileName);var target=Path.Combine(core.MediaDir,"custom_now_playing"+ext);DeletePrefix(core.MediaDir,"custom_now_playing",target);await File.WriteAllBytesAsync(target,f.Data,ct);await core.SetCustomImageAsync(Path.GetFileName(target),ct);return J(core.Snapshot());});
        web.MapPost("/api/remove-image",async (CancellationToken ct)=>{await core.RemoveCustomImageAsync(ct);return J(core.Snapshot());});
        web.MapPost("/api/upload-background",async (HttpRequest r,CancellationToken ct)=>{var f=await FormFile(r,"background",AppUtil.MaxUploadBytes,ct,"file");var ext=ValidateMedia(f.Data,f.FileName);var target=Path.Combine(core.MediaDir,"custom_background"+ext);DeletePrefix(core.MediaDir,"custom_background",target);await File.WriteAllBytesAsync(target,f.Data,ct);await core.SetBackgroundAsync(Path.GetFileName(target),ct);return J(core.Snapshot());});
        web.MapPost("/api/remove-background",async (CancellationToken ct)=>{await core.RemoveBackgroundAsync(ct);return J(core.Snapshot());});
        web.MapMethods("/media/custom",["GET","HEAD"],(HttpContext c)=>core.CurrentCustomImagePath() is string p?FileResult(p,c):Results.NotFound());
        web.MapMethods("/media/background",["GET","HEAD"],(HttpContext c)=>core.CurrentBackgroundPath() is string p?FileResult(p,c):Results.NotFound());
        web.MapPost("/api/upload-font",async (HttpRequest r,CancellationToken ct)=>{var family=await SaveFontAsync(r,ct);var s=core.SettingsSnapshot();s.TextFont=family;await core.ApplySettingsAsync(s,ct);return J(core.Snapshot());});
        web.MapPost("/api/remove-font",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);var family=d.RootElement.TryGetProperty("family",out var x)?x.GetString()?.Trim()??"":"";var info=core.ListFonts().FirstOrDefault(f=>f.Family==family)??throw new InvalidDataException("select an uploaded custom font first");try{File.Delete(Path.Combine(core.FontDir,Path.GetFileName(info.ID)));}catch{}var s=core.SettingsSnapshot();if(s.TextFont==family){s.TextFont="Segoe UI";await core.ApplySettingsAsync(s,ct);}var ch=chat.State().Settings;if(ch.FontFamily==family){ch.FontFamily="Segoe UI";await chat.SetSettingsAsync(ch);}var cd=countdown.State().Settings;if(cd.FontFamily==family){cd.FontFamily="Segoe UI";await countdown.ApplySettingsAsync(cd);}return J(core.Snapshot());});
        web.MapMethods("/font",["GET","HEAD"],(HttpContext c)=>{var name=Path.GetFileName(c.Request.Query["name"].ToString());if(name.Length==0||!FontExt(Path.GetExtension(name)))return Results.NotFound();return FileResult(Path.Combine(core.FontDir,name),c,FontType(Path.GetExtension(name)));});
        web.MapPost("/api/open-folder",()=>{AppUtil.OpenFolder(core.DataDir);return Results.NoContent();});
        web.MapGet("/api/update",async (CancellationToken ct)=>J(await updates.CheckAsync(ct)));
        web.MapPost("/api/update/open",()=>{AppUtil.OpenExternal(UpdateService.RepositoryURL);return Results.NoContent();});
        web.MapPost("/api/overlay-metrics",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);var fps=Num(d.RootElement,"fps");var fm=Num(d.RootElement,"frame_ms");core.SetOverlayMetrics(fps,fm);return Results.NoContent();});
    }

    private void MapProfiles(WebApplication web)
    {
        web.MapGet("/api/profiles",()=>J(core.ListProfiles()));
        web.MapPost("/api/profiles",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);var a=Str(d.RootElement,"action");var n=Str(d.RootElement,"name");var nn=Str(d.RootElement,"new_name");string result=n;switch(a){case"save":await profiles.SaveProfileAsync(n);break;case"load":await profiles.LoadProfileAsync(n);break;case"duplicate":result=await profiles.DuplicateProfileAsync(n,nn);break;case"rename":result=AppUtil.SanitizeProfileName(nn);await profiles.RenameProfileAsync(n,result);break;case"set_default":await profiles.SetDefaultAsync(n);break;case"clear_default":result="";await profiles.SetDefaultAsync("");break;case"delete":await profiles.DeleteProfileAsync(n);break;default:throw new InvalidDataException("unknown profile action");}var state=core.Snapshot();return a is "duplicate" or "rename" or "set_default" or "clear_default" or "delete"?J(new{state,name=result}):J(state);});
        web.MapGet("/api/profile-export",(HttpRequest r)=>{var name=r.Query["name"].ToString();return Results.File(profiles.ExportProfile(name),"application/zip",AppUtil.SanitizeProfileName(name)+".sleepyprofile.zip");});
        web.MapPost("/api/profile-import",async (HttpRequest r,CancellationToken ct)=>{var form=await r.ReadFormAsync(ct);var file=form.Files.GetFile("bundle")??throw new InvalidDataException("choose a SleepySource profile bundle first");var bytes=await ReadFile(file,AppUtil.MaxProfileBundleBytes,ct);var overwrite=form["overwrite"].ToString()=="1";var name=await profiles.ImportProfileAsync(bytes,file.FileName,overwrite);return J(new{name,profiles=core.ListProfiles()});});
    }

    private void MapCountdown(WebApplication web)
    {
        web.MapMethods("/api/countdown/state",["GET","HEAD"],()=>J(countdown.State(core.ListFonts())));
        web.MapMethods("/api/countdown/settings",["GET","HEAD"],()=>J(countdown.State(core.ListFonts())));
        web.MapPost("/api/countdown/settings",async (HttpRequest r,CancellationToken ct)=>{var s=await ReadJson<CountdownSettings>(r,ct)??throw new InvalidDataException("invalid countdown settings");await countdown.ApplySettingsAsync(s);return J(countdown.State(core.ListFonts()));});
        web.MapPost("/api/countdown/control",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);countdown.Control(Str(d.RootElement,"action"));return J(countdown.State(core.ListFonts()));});
        web.MapMethods("/api/countdown/profiles",["GET","HEAD"],()=>J(countdown.State(core.ListFonts())));
        web.MapPost("/api/countdown/profiles",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);var a=Str(d.RootElement,"action");var n=Str(d.RootElement,"name");switch(a){case"save":await countdown.SaveProfileAsync(n);break;case"load":await countdown.LoadProfileAsync(n);break;case"delete":countdown.DeleteProfile(n);break;default:throw new InvalidDataException("unknown countdown profile action");}return J(countdown.State(core.ListFonts()));});
        web.MapPost("/api/countdown/upload-font",async (HttpRequest r,CancellationToken ct)=>{var fam=await SaveFontAsync(r,ct);var s=countdown.State().Settings;s.FontFamily=fam;await countdown.ApplySettingsAsync(s);return J(countdown.State(core.ListFonts()));});
    }

    private void MapAlerts(WebApplication web)
    {
        web.MapMethods("/api/alerts/state",["GET","HEAD"],(HttpRequest r)=>J(alerts.State(string.Equals(r.Query["consumer"].ToString(),"overlay",StringComparison.OrdinalIgnoreCase))));
        web.MapMethods("/api/alerts/settings",["GET","HEAD"],(HttpRequest r)=>{var d=r.Query["defaults"].ToString().Trim();return J(d=="1"||d.Equals("true",StringComparison.OrdinalIgnoreCase)?AlertService.DefaultSettings():alerts.SettingsSnapshot());});
        web.MapPost("/api/alerts/settings",async (HttpRequest r,CancellationToken ct)=>{var s=await ReadJson<AlertSettings>(r,ct)??throw new InvalidDataException("invalid alert settings");await alerts.SetSettingsAsync(s);return J(alerts.SettingsSnapshot());});
        web.MapPost("/api/alerts/test",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);var type=Str(d.RootElement,"type");var ev=alerts.Sample(type);var u=Str(d.RootElement,"username");if(u.Length>0)ev.Username=u;var n=Int(d.RootElement,"amount");if(n>0)ev.Amount=n;n=Int(d.RootElement,"count");if(n>0)ev.Count=n;n=Int(d.RootElement,"months");if(n>0)ev.Months=n;var x=Str(d.RootElement,"tier");if(x.Length>0)ev.Tier=x;x=Str(d.RootElement,"gift_name");if(x.Length>0)ev.GiftName=x;x=Str(d.RootElement,"reward_title");if(x.Length>0)ev.RewardTitle=x;x=Str(d.RootElement,"user_input");if(x.Length>0)ev.UserInput=x;ev.ID=$"test-{type}-{AppUtil.NowMS()}";if(!alerts.Enqueue(ev))return Results.Conflict("alert is disabled or the queue is full");return J(alerts.State(false));});
        web.MapPost("/api/alerts/control",async (HttpRequest r,CancellationToken ct)=>{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);alerts.Control(Str(d.RootElement,"action"));return J(alerts.State(false));});
        web.MapPost("/api/alerts/upload",async (HttpRequest r,CancellationToken ct)=>{var type=r.Query["type"].ToString();var kind=r.Query["kind"].ToString().ToLowerInvariant();if(!AlertService.AlertTypes.Contains(type)||(kind!="visual"&&kind!="sound"))throw new InvalidDataException("invalid alert media target");var f=await FormFile(r,"file",AppUtil.MaxUploadBytes,ct);var ext=kind=="visual"?ValidateMedia(f.Data,f.FileName):ValidateSound(f.Data,f.FileName);var dir=Path.Combine(alerts.MediaDir,type);Directory.CreateDirectory(dir);var target=Path.Combine(dir,kind+ext);DeletePrefix(dir,kind,target);await File.WriteAllBytesAsync(target,f.Data,ct);await alerts.SetMediaAsync(type,kind,Path.GetFileName(target));if(kind=="visual"){var s=alerts.SettingsSnapshot();var st=s.Types[type];if(st.DisplayMode is "card" or "" or "text-only"){st.DisplayMode="custom";st.MediaX=0;st.MediaY=0;st.MediaWidth=st.Width;st.MediaHeight=st.Height;st.MediaFit="contain";st.MediaOpacity=100;st.TitleX=40;st.TitleY=Math.Max(10,st.Height-165);st.TitleWidth=Math.Max(80,st.Width-80);st.TitleHeight=100;st.TitleAlign="center";st.MessageX=40;st.MessageY=Math.Max(10,st.Height-62);st.MessageWidth=Math.Max(80,st.Width-80);st.MessageHeight=52;st.MessageAlign="center";s.Types[type]=st;await alerts.SetSettingsAsync(s);}}return J(alerts.State(false));});
        web.MapPost("/api/alerts/remove-media",async (HttpRequest r)=>{var type=r.Query["type"].ToString();var kind=r.Query["kind"].ToString().ToLowerInvariant();if(!AlertService.AlertTypes.Contains(type)||(kind!="visual"&&kind!="sound"))throw new InvalidDataException("invalid alert media target");await alerts.RemoveMediaAsync(type,kind);return J(alerts.State(false));});
        web.MapMethods("/media/alerts",["GET","HEAD"],(HttpContext c)=>{var p=alerts.MediaPath(c.Request.Query["type"].ToString(),c.Request.Query["kind"].ToString());return p is null?Results.NotFound():FileResult(p,c);});
    }

    private void MapChat(WebApplication web)
    {
        web.MapMethods("/api/chat/state",["GET","HEAD"],()=>J(ResponsePayloads.Chat(chat.State())));
        web.MapGet("/api/chat/settings",()=>J(chat.State().Settings));
        web.MapPost("/api/chat/settings",async (HttpRequest r,CancellationToken ct)=>{var s=await ReadJson<ChatSettings>(r,ct)??throw new InvalidDataException("invalid settings");await chat.SetSettingsAsync(s);return J(chat.State().Settings);});
        web.MapPost("/api/chat/upload-font",async (HttpRequest r,CancellationToken ct)=>{var fam=await SaveFontAsync(r,ct);var s=chat.State().Settings;s.FontFamily=fam;await chat.SetSettingsAsync(s);return J(new{settings=chat.State().Settings,fonts=core.ListFonts()});});
        web.MapPost("/api/chat/test",async (HttpRequest r,CancellationToken ct)=>{JsonElement root=default;try{using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);root=d.RootElement.Clone();}catch{}var username=root.ValueKind==JsonValueKind.Object?Str(root,"username"):"";var text=root.ValueKind==JsonValueKind.Object?Str(root,"text"):"";var uid=root.ValueKind==JsonValueKind.Object?Str(root,"user_id"):"";chat.AddMessage(new ChatMessage{UserID=uid.Length>0?uid:"123456",Username=username.Length>0?username:"SleepyViewer",Text=text.Length>0?text:"This is a test chat message OMEGALUL",Color="#55B7FF",IsMod=true,Badges=["Moderator","Subscriber"],BadgeDetails=[new(){Text="Moderator",Type="moderator"},new(){Text="Subscriber",Type="subscriber",Count=3}]});return J(ResponsePayloads.Chat(chat.State()));});
        web.MapPost("/api/chat/clear",()=>{chat.ClearMessages();return Results.NoContent();});
        web.MapPost("/api/chat/ingest",async (HttpRequest r,CancellationToken ct)=>{var bytes=await ReadLimited(r.Body,1L<<20,ct);if(bytes.Length==0)throw new InvalidDataException("invalid chat message");using var d=JsonDocument.Parse(bytes);var msg=ChatService.ParseKickChat(d.RootElement)??JsonSerializer.Deserialize<ChatMessage>(bytes,AppUtil.Json);if(msg is null||string.IsNullOrWhiteSpace(msg.Text))throw new InvalidDataException("message text required");chat.AddMessage(msg);return Results.NoContent();});
        web.MapGet("/api/chat/7tv",async (HttpRequest r,CancellationToken ct)=>J(await kickAssets.SevenTVAsync(r.Query["emote_set_id"],r.Query["kick_channel"],ct)));
        web.MapGet("/api/chat/7tv-image",async (HttpRequest r,CancellationToken ct)=>{var x=await kickAssets.SevenTVImageAsync(r.Query["id"]!,ct);return Results.File(x.Data,x.ContentType);});
        web.MapGet("/api/chat/kick-emote",async (HttpRequest r,CancellationToken ct)=>{var x=await kickAssets.KickEmoteAsync(r.Query["id"]!,ct);return Results.File(x.Data,x.ContentType);});
        web.MapGet("/api/chat/avatar",async (HttpRequest r,CancellationToken ct)=>{var x=await kickAssets.AvatarAsync(r.Query["url"]!,ct);return Results.File(x.Data,x.ContentType);});
        web.MapGet("/api/chat/badges",async (HttpRequest r,CancellationToken ct)=>J(await kickAssets.BadgeCatalogAsync(r.Query["channel"],ct)));
        web.MapGet("/api/chat/badge-image",async (HttpRequest r,CancellationToken ct)=>{int.TryParse(r.Query["count"],out var count);var x=await kickAssets.BadgeImageAsync(r.Query["url"],r.Query["role"],count,ct);return Results.File(x.Data,x.ContentType);});
    }

    private void MapKick(WebApplication web)
    {
        web.MapGet("/api/kick/status",()=>J(sleepyApi.State()));
        web.MapPost("/api/kick/oauth/start",async (CancellationToken ct)=>J(await sleepyApi.BeginOAuthAsync(ct)));
        web.MapPost("/api/kick/oauth/status",async (CancellationToken ct)=>J(await sleepyApi.PollOAuthAsync(ct)));
        web.MapPost("/api/kick/refresh",async (CancellationToken ct)=>{await sleepyApi.RefreshConnectionAsync(ct);return J(sleepyApi.State());});
        web.MapPost("/api/kick/events/sync",async (CancellationToken ct)=>J(await sleepyApi.SyncEventsAsync(ct)));
        web.MapPost("/api/kick/disconnect",async (CancellationToken ct)=>J(await sleepyApi.DisconnectAsync(ct)));
        web.MapGet("/api/stream/categories",async (HttpRequest r,CancellationToken ct)=>J(await sleepyApi.SearchCategoriesAsync(r.Query["q"].ToString(),ct)));
        web.MapGet("/api/stream/metadata",async (CancellationToken ct)=>J(await sleepyApi.StreamMetadataAsync(ct)));
        web.MapPost("/api/stream/update",async (HttpRequest r,CancellationToken ct)=>
        {
            using var d=await JsonDocument.ParseAsync(r.Body,cancellationToken:ct);
            var title=Str(d.RootElement,"title");
            if(title.Length==0)throw new InvalidDataException("Stream title is required");
            return J(await sleepyApi.UpdateStreamAsync(title,Long(d.RootElement,"category_id"),Str(d.RootElement,"category_name"),ct));
        });
    }

    private void MapHealth(WebApplication web)
    {
        web.MapGet("/api/system/health",async (CancellationToken ct)=>J(await health.RunAsync(ct)));
        web.MapMethods("/api/health",["GET","HEAD"],(HttpContext c)=>HttpMethods.IsHead(c.Request.Method)?Results.Ok():Results.Text("ok","text/plain"));
    }

    private async Task<string> SaveFontAsync(HttpRequest r,CancellationToken ct)
    {
        var f=await FormFile(r,"font",AppUtil.MaxFontBytes,ct);var ext=ValidateFont(f.Data,f.FileName);var baseName=AppUtil.SanitizeFontBase(f.FileName);var name=baseName+ext;var target=Path.Combine(core.FontDir,name);DeletePrefix(core.FontDir,baseName,target);await File.WriteAllBytesAsync(target,f.Data,ct);return "NPF_"+AppUtil.SanitizeFontBase(name);
    }
    private static async Task<(byte[] Data,string FileName)> FormFile(HttpRequest r,string name,long max,CancellationToken ct,string? alt=null){var form=await r.ReadFormAsync(ct);var f=form.Files.GetFile(name)??(!string.IsNullOrWhiteSpace(alt)?form.Files.GetFile(alt):null)??throw new InvalidDataException("choose a file first");return(await ReadFile(f,max,ct),f.FileName);}
    private static async Task<byte[]> ReadFile(IFormFile f,long max,CancellationToken ct){if(f.Length>max)throw new InvalidDataException("file is too large");await using var s=f.OpenReadStream();return await ReadLimited(s,max,ct);}
    private static async Task<byte[]> ReadLimited(Stream s,long max,CancellationToken ct){using var ms=new MemoryStream();var b=new byte[81920];long total=0;while(true){var n=await s.ReadAsync(b,ct);if(n<=0)break;total+=n;if(total>max)throw new InvalidDataException("file is too large");ms.Write(b,0,n);}return ms.ToArray();}
    private static async Task<T?> ReadJson<T>(HttpRequest r,CancellationToken ct)=>await r.ReadFromJsonAsync<T>(AppUtil.Json,ct);
    private static IResult J(object? value,int status=200)=>Results.Json(value,AppUtil.Json,statusCode:status);
    private static string Str(JsonElement r,string p)=>r.ValueKind==JsonValueKind.Object&&r.TryGetProperty(p,out var x)?x.ToString().Trim():"";
    private static int Int(JsonElement r,string p)=>int.TryParse(Str(r,p),out var n)?n:0;
    private static long Long(JsonElement r,string p)=>long.TryParse(Str(r,p),out var n)?n:0;
    private static double Num(JsonElement r,string p)=>double.TryParse(Str(r,p),System.Globalization.NumberStyles.Any,System.Globalization.CultureInfo.InvariantCulture,out var n)?n:0;
    private static bool Bool(JsonElement r,string p){if(r.ValueKind!=JsonValueKind.Object||!r.TryGetProperty(p,out var x))return false;return x.ValueKind==JsonValueKind.True||(x.ValueKind==JsonValueKind.String&&bool.TryParse(x.GetString(),out var b)&&b);}
    private static bool FontExt(string ext)=>new[]{".ttf",".otf",".woff",".woff2"}.Contains(ext.ToLowerInvariant());
    private static string FontType(string ext)=>ext.ToLowerInvariant() switch{".ttf"=>"font/ttf",".otf"=>"font/otf",".woff"=>"font/woff",".woff2"=>"font/woff2",_=>"application/octet-stream"};
    private static string ValidateFont(byte[] d,string file){var ext=Path.GetExtension(file).ToLowerInvariant();if(!FontExt(ext))throw new InvalidDataException("use a TTF, OTF, WOFF, or WOFF2 font file");var ok=ext switch{".ttf"=>d.Length>=4&&((d[0]==0&&d[1]==1&&d[2]==0&&d[3]==0)||Encoding.ASCII.GetString(d,0,4)=="true"),".otf"=>d.Length>=4&&Encoding.ASCII.GetString(d,0,4)=="OTTO",".woff"=>d.Length>=4&&Encoding.ASCII.GetString(d,0,4)=="wOFF",".woff2"=>d.Length>=4&&Encoding.ASCII.GetString(d,0,4)=="wOF2",_=>false};if(!ok)throw new InvalidDataException("the selected file does not look like a valid font");return ext;}
    private static string ValidateMedia(byte[] d,string file){if(d.Length>=8&&d[0]==0x89&&Encoding.ASCII.GetString(d,1,3)=="PNG")return".png";if(d.Length>=3&&d[0]==0xff&&d[1]==0xd8&&d[2]==0xff)return".jpg";if(d.Length>=6&&(Encoding.ASCII.GetString(d,0,6)=="GIF87a"||Encoding.ASCII.GetString(d,0,6)=="GIF89a"))return".gif";if(d.Length>=12&&Encoding.ASCII.GetString(d,0,4)=="RIFF"&&Encoding.ASCII.GetString(d,8,4)=="WEBP")return".webp";if(Path.GetExtension(file).Equals(".webm",StringComparison.OrdinalIgnoreCase)&&d.Length>=4&&d[0]==0x1A&&d[1]==0x45&&d[2]==0xDF&&d[3]==0xA3)return".webm";throw new InvalidDataException("use a PNG, JPG, GIF, WEBP, or WEBM file");}
    private static string ValidateSound(byte[] d,string file){var ext=Path.GetExtension(file).ToLowerInvariant();if(d.Length>=12&&Encoding.ASCII.GetString(d,0,4)=="RIFF"&&Encoding.ASCII.GetString(d,8,4)=="WAVE"&&ext==".wav")return".wav";if(d.Length>=4&&Encoding.ASCII.GetString(d,0,4)=="OggS"&&ext==".ogg")return".ogg";if(d.Length>=3&&Encoding.ASCII.GetString(d,0,3)=="ID3"&&ext==".mp3")return".mp3";if(d.Length>=2&&d[0]==0xff&&(d[1]&0xe0)==0xe0&&ext==".mp3")return".mp3";if(d.Length>=12&&Encoding.ASCII.GetString(d,4,4)=="ftyp"&&(ext==".m4a"||ext==".mp4"))return".m4a";throw new InvalidDataException("use an MP3, WAV, OGG, or M4A sound file");}
    private static void DeletePrefix(string dir,string prefix,string except){Directory.CreateDirectory(dir);foreach(var f in Directory.EnumerateFiles(dir,prefix+".*"))if(!Path.GetFullPath(f).Equals(Path.GetFullPath(except),StringComparison.OrdinalIgnoreCase))try{File.Delete(f);}catch{}}

    public void Dispose(){Stop();media.Dispose();kickAssets.Dispose();sleepyApi.Dispose();updates.Dispose();}
}
