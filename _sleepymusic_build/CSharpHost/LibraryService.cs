using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using TagLibSharp2.Core;

namespace SleepyMusic;

internal sealed class LibraryService
{
    private static readonly HashSet<string> SupportedExtensions = new(StringComparer.OrdinalIgnoreCase)
    { ".mp3", ".flac", ".wav", ".m4a", ".aac", ".ogg", ".opus", ".wma", ".aiff", ".aif", ".ape", ".wv", ".mpc" };
    private readonly SemaphoreSlim gate = new(1,1);
    private readonly string settingsPath = Path.Combine(AppUtil.DataDir,"settings.json");
    private readonly string libraryPath = Path.Combine(AppUtil.DataDir,"library.json");
    private MusicSettings settings = new();
    private List<MusicTrack> tracks = [];

    public LibraryService() => Load();
    public MusicSettings GetSettingsSnapshot() { lock(settings) return Clone(settings); }
    public List<MusicTrack> GetTracksSnapshot() { lock(tracks) return tracks.Select(CloneTrack).ToList(); }
    public MusicTrack? FindTrack(string id) { lock(tracks) return tracks.FirstOrDefault(t=>t.Id==id) is { } t ? CloneTrack(t) : null; }

    public async Task AddFolderAsync(string path, CancellationToken ct=default)
    {
        if(string.IsNullOrWhiteSpace(path)||!Directory.Exists(path)) return;
        var normalized=Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar,Path.AltDirectorySeparatorChar);
        await gate.WaitAsync(ct); try { if(!settings.LibraryFolders.Any(x=>string.Equals(x,normalized,StringComparison.OrdinalIgnoreCase))) settings.LibraryFolders.Add(normalized); SaveSettingsUnsafe(); } finally { gate.Release(); }
        await RescanAsync(ct);
    }
    public async Task RemoveFolderAsync(string path,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { settings.LibraryFolders.RemoveAll(x=>string.Equals(x,path,StringComparison.OrdinalIgnoreCase)); SaveSettingsUnsafe(); } finally { gate.Release(); }
        await RescanAsync(ct);
    }
    public async Task RescanAsync(CancellationToken ct=default)
    {
        await gate.WaitAsync(ct);
        try
        {
            var found=new List<MusicTrack>();
            foreach(var folder in settings.LibraryFolders.Where(Directory.Exists).Distinct(StringComparer.OrdinalIgnoreCase))
                foreach(var path in EnumerateSafe(folder)) { ct.ThrowIfCancellationRequested(); if(!SupportedExtensions.Contains(Path.GetExtension(path))) continue; try { found.Add(ReadTrack(path)); } catch { } }
            tracks=found.GroupBy(t=>t.Id,StringComparer.Ordinal).Select(g=>g.First()).OrderBy(t=>t.Artist,StringComparer.CurrentCultureIgnoreCase).ThenBy(t=>t.Album,StringComparer.CurrentCultureIgnoreCase).ThenBy(t=>t.Title,StringComparer.CurrentCultureIgnoreCase).ToList();
            var valid=tracks.Select(t=>t.Id).ToHashSet(StringComparer.Ordinal);
            settings.FavoriteTrackIds.RemoveAll(id=>!valid.Contains(id));
            foreach(var p in settings.Playlists) p.TrackIds.RemoveAll(id=>!valid.Contains(id));
            settings.RecentlyPlayedIds.RemoveAll(id=>!valid.Contains(id));
            if(settings.LastTrackId is not null&&!valid.Contains(settings.LastTrackId)) settings.LastTrackId=null;
            SaveLibraryUnsafe(); SaveSettingsUnsafe();
        }
        finally { gate.Release(); }
    }
    public async Task SetFavoriteAsync(string id,bool favorite,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { settings.FavoriteTrackIds.RemoveAll(x=>x==id); if(favorite&&tracks.Any(t=>t.Id==id)) settings.FavoriteTrackIds.Insert(0,id); SaveSettingsUnsafe(); } finally { gate.Release(); }
    }
    public async Task MarkPlayedAsync(string id,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { if(!tracks.Any(t=>t.Id==id)) return; settings.LastTrackId=id; settings.RecentlyPlayedIds.RemoveAll(x=>x==id); settings.RecentlyPlayedIds.Insert(0,id); if(settings.RecentlyPlayedIds.Count>40) settings.RecentlyPlayedIds.RemoveRange(40,settings.RecentlyPlayedIds.Count-40); SaveSettingsUnsafe(); } finally { gate.Release(); }
    }
    public async Task UpdatePreferencesAsync(string? theme,double? volume,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { if(theme is "blue" or "red" or "purple" or "green" or "pink") settings.Theme=theme; if(volume.HasValue) settings.Volume=Math.Clamp(volume.Value,0,1); SaveSettingsUnsafe(); } finally { gate.Release(); }
    }
    public async Task<MusicPlaylist> CreatePlaylistAsync(string name,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { var clean=string.IsNullOrWhiteSpace(name)?"New Playlist":name.Trim(); var p=new MusicPlaylist{Name=clean.Length>60?clean[..60]:clean}; settings.Playlists.Add(p); SaveSettingsUnsafe(); return p; } finally { gate.Release(); }
    }
    public async Task DeletePlaylistAsync(string id,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { settings.Playlists.RemoveAll(p=>p.Id==id); SaveSettingsUnsafe(); } finally { gate.Release(); }
    }
    public async Task SetPlaylistTrackAsync(string playlistId,string trackId,bool include,CancellationToken ct=default)
    {
        await gate.WaitAsync(ct); try { var p=settings.Playlists.FirstOrDefault(x=>x.Id==playlistId); if(p is null||!tracks.Any(t=>t.Id==trackId)) return; p.TrackIds.RemoveAll(x=>x==trackId); if(include)p.TrackIds.Add(trackId); SaveSettingsUnsafe(); } finally { gate.Release(); }
    }

    private static IEnumerable<string> EnumerateSafe(string root)
    {
        var pending=new Stack<string>(); pending.Push(root);
        while(pending.Count>0) { var dir=pending.Pop(); string[] files,dirs; try{files=Directory.GetFiles(dir);}catch{files=[];} try{dirs=Directory.GetDirectories(dir);}catch{dirs=[];} foreach(var f in files)yield return f; foreach(var d in dirs)pending.Push(d); }
    }
    private static MusicTrack ReadTrack(string path)
    {
        var info=new FileInfo(path); var title=Path.GetFileNameWithoutExtension(path); var artist="Unknown Artist"; var album="Unknown Album"; uint? year=null; var genre="";
        try { var result=MediaFile.Read(path); if(result.IsSuccess&&result.Tag is { } tag) { if(!string.IsNullOrWhiteSpace(tag.Title))title=tag.Title.Trim(); if(!string.IsNullOrWhiteSpace(tag.Artist))artist=tag.Artist.Trim(); if(!string.IsNullOrWhiteSpace(tag.Album))album=tag.Album.Trim(); if(tag.Year>0)year=tag.Year; if(!string.IsNullOrWhiteSpace(tag.Genre))genre=tag.Genre.Trim(); } } catch { }
        return new MusicTrack { Id=StableId(path),Path=Path.GetFullPath(path),FileName=info.Name,Title=title,Artist=artist,Album=album,Year=year,Genre=genre,SizeBytes=info.Length,ModifiedUtc=info.LastWriteTimeUtc,Extension=info.Extension.TrimStart('.').ToUpperInvariant() };
    }
    private static string StableId(string path) { var hash=SHA256.HashData(Encoding.UTF8.GetBytes(Path.GetFullPath(path).ToUpperInvariant())); return Convert.ToHexString(hash.AsSpan(0,12)).ToLowerInvariant(); }
    private void Load()
    {
        try { if(File.Exists(settingsPath)) settings=JsonSerializer.Deserialize<MusicSettings>(File.ReadAllText(settingsPath),AppUtil.Json)??new(); } catch { settings=new(); }
        try { if(File.Exists(libraryPath)) tracks=JsonSerializer.Deserialize<List<MusicTrack>>(File.ReadAllText(libraryPath),AppUtil.Json)??[]; } catch { tracks=[]; }
        settings.LibraryFolders=settings.LibraryFolders.Where(Directory.Exists).Distinct(StringComparer.OrdinalIgnoreCase).ToList();
    }
    private void SaveSettingsUnsafe()=>AtomicWrite(settingsPath,JsonSerializer.Serialize(settings,AppUtil.Json));
    private void SaveLibraryUnsafe()=>AtomicWrite(libraryPath,JsonSerializer.Serialize(tracks,AppUtil.Json));
    private static void AtomicWrite(string path,string text){Directory.CreateDirectory(Path.GetDirectoryName(path)!);var tmp=path+".tmp";File.WriteAllText(tmp,text,Encoding.UTF8);File.Move(tmp,path,true);}
    private static T Clone<T>(T value)=>JsonSerializer.Deserialize<T>(JsonSerializer.Serialize(value,AppUtil.Json),AppUtil.Json)!;
    private static MusicTrack CloneTrack(MusicTrack t)=>new(){Id=t.Id,Path=t.Path,FileName=t.FileName,Title=t.Title,Artist=t.Artist,Album=t.Album,Year=t.Year,Genre=t.Genre,SizeBytes=t.SizeBytes,ModifiedUtc=t.ModifiedUtc,Extension=t.Extension};
}
