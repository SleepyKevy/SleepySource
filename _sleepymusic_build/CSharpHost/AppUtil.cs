using System.Text.Json;
using System.Text.Json.Serialization;

namespace SleepyMusic;

internal static class AppUtil
{
    public const string Version = "1.0.0";
    public static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web)
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        WriteIndented = true
    };

    public static string DataDir
    {
        get
        {
            var dir = Path.Combine(AppContext.BaseDirectory, "SleepyMusic_Data");
            Directory.CreateDirectory(dir);
            return dir;
        }
    }

    public static string RuntimeDataDir
    {
        get
        {
            var dir = Path.Combine(DataDir, "WebView2");
            Directory.CreateDirectory(dir);
            return dir;
        }
    }

    public static string ContentTypeForPath(string path) => Path.GetExtension(path).ToLowerInvariant() switch
    {
        ".html" => "text/html; charset=utf-8", ".json" or ".webmanifest" => "application/json; charset=utf-8",
        ".png" => "image/png", ".webp" => "image/webp", ".ico" => "image/x-icon", ".svg" => "image/svg+xml",
        ".mp3" => "audio/mpeg", ".flac" => "audio/flac", ".wav" => "audio/wav", ".m4a" or ".aac" => "audio/mp4",
        ".ogg" or ".opus" => "audio/ogg", ".wma" => "audio/x-ms-wma", ".aiff" or ".aif" => "audio/aiff",
        _ => "application/octet-stream"
    };
}
