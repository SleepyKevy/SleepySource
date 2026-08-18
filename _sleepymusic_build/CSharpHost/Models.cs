namespace SleepyMusic;

internal sealed class MusicTrack
{
    public string Id { get; set; } = "";
    public string Path { get; set; } = "";
    public string FileName { get; set; } = "";
    public string Title { get; set; } = "";
    public string Artist { get; set; } = "Unknown Artist";
    public string Album { get; set; } = "Unknown Album";
    public uint? Year { get; set; }
    public string Genre { get; set; } = "";
    public long SizeBytes { get; set; }
    public DateTime ModifiedUtc { get; set; }
    public string Extension { get; set; } = "";
}

internal sealed class MusicPlaylist
{
    public string Id { get; set; } = Guid.NewGuid().ToString("N");
    public string Name { get; set; } = "New Playlist";
    public List<string> TrackIds { get; set; } = [];
}

internal sealed class MusicSettings
{
    public List<string> LibraryFolders { get; set; } = [];
    public List<string> FavoriteTrackIds { get; set; } = [];
    public List<MusicPlaylist> Playlists { get; set; } = [];
    public string Theme { get; set; } = "blue";
    public double Volume { get; set; } = 0.72;
    public string? LastTrackId { get; set; }
    public List<string> RecentlyPlayedIds { get; set; } = [];
}
