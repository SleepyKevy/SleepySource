using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using System.Diagnostics;

namespace SleepyMusic;

internal sealed class BackendHost : IDisposable
{
    public const int Port = 17893;
    public const string BaseUrl = "http://127.0.0.1:17893/";
    private WebApplication? app;
    public LibraryService Library { get; } = new();

    public async Task StartAsync(CancellationToken ct=default)
    {
        if(app is not null)return;
        var builder=WebApplication.CreateBuilder(new WebApplicationOptions{ContentRootPath=AppContext.BaseDirectory,ApplicationName=typeof(BackendHost).Assembly.GetName().Name});
        builder.WebHost.UseKestrel(k=>k.ListenLocalhost(Port));
        var web=builder.Build();
        web.Use(async(ctx,next)=>{ctx.Response.Headers.CacheControl="no-store";ctx.Response.Headers.XContentTypeOptions="nosniff";ctx.Response.Headers.ReferrerPolicy="no-referrer";ctx.Response.Headers.Append("Content-Security-Policy","default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; media-src 'self' blob:; connect-src 'self'; frame-src 'none'; object-src 'none'; base-uri 'none'");await next();});
        MapStatic(web);MapApi(web);app=web;await web.StartAsync(ct);
    }

    private static void MapStatic(WebApplication web)
    {
        var webDir=Path.Combine(AppContext.BaseDirectory,"web"),assetsDir=Path.Combine(AppContext.BaseDirectory,"assets");
        web.MapMethods("/",["GET","HEAD"],(HttpContext c)=>LocalFile(Path.Combine(webDir,"index.html"),c));
        web.MapMethods("/index.html",["GET","HEAD"],(HttpContext c)=>LocalFile(Path.Combine(webDir,"index.html"),c));
        web.MapMethods("/manifest.webmanifest",["GET","HEAD"],(HttpContext c)=>LocalFile(Path.Combine(webDir,"manifest.webmanifest"),c));
        web.MapMethods("/assets/{**name}",["GET","HEAD"],(string name,HttpContext c)=>{var root=Path.GetFullPath(assetsDir)+Path.DirectorySeparatorChar;var path=Path.GetFullPath(Path.Combine(assetsDir,name??""));return path.StartsWith(root,StringComparison.OrdinalIgnoreCase)?LocalFile(path,c):Results.BadRequest();});
    }
    private static IResult LocalFile(string path,HttpContext ctx)
    {
        if(!File.Exists(path))return Results.NotFound();
        if(HttpMethods.IsHead(ctx.Request.Method)){ctx.Response.ContentType=AppUtil.ContentTypeForPath(path);ctx.Response.ContentLength=new FileInfo(path).Length;return Results.Empty;}
        return Results.File(path,AppUtil.ContentTypeForPath(path),enableRangeProcessing:false);
    }

    private void MapApi(WebApplication web)
    {
        web.MapGet("/api/health",()=>Results.Text("ok"));
        web.MapGet("/api/status",()=>Results.Json(new{ok=true,name="SleepyMusic",version=AppUtil.Version,platform="windows",host="csharp-webview2"},AppUtil.Json));
        web.MapGet("/api/library",()=>Results.Json(new{tracks=Library.GetTracksSnapshot(),settings=Library.GetSettingsSnapshot()},AppUtil.Json));
        web.MapPost("/api/library/rescan",async(CancellationToken ct)=>{await Library.RescanAsync(ct);return Results.Json(new{ok=true,tracks=Library.GetTracksSnapshot()},AppUtil.Json);});
        web.MapPost("/api/library/remove-folder",async(FolderRequest b,CancellationToken ct)=>{await Library.RemoveFolderAsync(b.Path??"",ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapMethods("/api/audio/{id}",["GET","HEAD"],(string id,HttpContext ctx)=>{var t=Library.FindTrack(id);if(t is null||!File.Exists(t.Path))return Results.NotFound();if(HttpMethods.IsHead(ctx.Request.Method)){ctx.Response.ContentType=AppUtil.ContentTypeForPath(t.Path);ctx.Response.ContentLength=new FileInfo(t.Path).Length;ctx.Response.Headers.AcceptRanges="bytes";return Results.Empty;}return Results.File(t.Path,AppUtil.ContentTypeForPath(t.Path),enableRangeProcessing:true);});
        web.MapPost("/api/favorite",async(FavoriteRequest b,CancellationToken ct)=>{await Library.SetFavoriteAsync(b.Id??"",b.Favorite,ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapPost("/api/played",async(IdRequest b,CancellationToken ct)=>{await Library.MarkPlayedAsync(b.Id??"",ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapPost("/api/preferences",async(PreferenceRequest b,CancellationToken ct)=>{await Library.UpdatePreferencesAsync(b.Theme,b.Volume,ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapPost("/api/playlists",async(PlaylistCreateRequest b,CancellationToken ct)=>Results.Json(await Library.CreatePlaylistAsync(b.Name??"",ct),AppUtil.Json));
        web.MapDelete("/api/playlists/{id}",async(string id,CancellationToken ct)=>{await Library.DeletePlaylistAsync(id,ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapPost("/api/playlists/{id}/track",async(string id,PlaylistTrackRequest b,CancellationToken ct)=>{await Library.SetPlaylistTrackAsync(id,b.TrackId??"",b.Include,ct);return Results.Json(new{ok=true},AppUtil.Json);});
        web.MapPost("/api/open-data",()=>{Directory.CreateDirectory(AppUtil.DataDir);try{Process.Start(new ProcessStartInfo("explorer.exe",AppUtil.DataDir){UseShellExecute=true});}catch{}return Results.Json(new{ok=true},AppUtil.Json);});
    }
    public void Stop(){var current=app;app=null;if(current is null)return;try{current.StopAsync(TimeSpan.FromSeconds(1)).GetAwaiter().GetResult();}catch{}try{current.DisposeAsync().AsTask().GetAwaiter().GetResult();}catch{}}
    public void Dispose()=>Stop();
    private sealed record FolderRequest(string? Path);
    private sealed record FavoriteRequest(string? Id,bool Favorite);
    private sealed record IdRequest(string? Id);
    private sealed record PreferenceRequest(string? Theme,double? Volume);
    private sealed record PlaylistCreateRequest(string? Name);
    private sealed record PlaylistTrackRequest(string? TrackId,bool Include);
}
