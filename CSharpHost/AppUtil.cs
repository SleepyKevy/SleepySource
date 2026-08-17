using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;

namespace SleepySource;

internal static class AppUtil
{
    public const string DisplayVersion = "1.1";
    public const string ListenPrefix = "http://127.0.0.1:17891/";
    public const long MaxUploadBytes = 50L << 20;
    public const long MaxFontBytes = 20L << 20;
    public const long MaxProfileBundleBytes = 160L << 20;
    public const long MaxBackupBytes = 500L << 20;
    public const long MaxBackupUploadBytes = MaxBackupBytes;
    public const long MaxBackupExtractBytes = 1L << 30;
    public const int MaxBackupFiles = 10_000;

    public static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
        WriteIndented = false,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.Never
    };

    public static readonly JsonSerializerOptions JsonIndented = new(Json)
    {
        WriteIndented = true
    };

    public static long NowMS() => DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
    public static long NowSeconds() => DateTimeOffset.UtcNow.ToUnixTimeSeconds();

    public static int Clamp(int value, int min, int max) => Math.Min(max, Math.Max(min, value));
    public static long Clamp(long value, long min, long max) => Math.Min(max, Math.Max(min, value));
    public static double Clamp(double value, double min, double max) => Math.Min(max, Math.Max(min, value));

    public static string NormalizeColor(string? value, string fallback)
    {
        value = (value ?? "").Trim();
        if (Regex.IsMatch(value, "^#[0-9A-Fa-f]{6}$")) return value.ToUpperInvariant();
        return fallback;
    }

    public static string SanitizeProfileName(string? name)
    {
        name = (name ?? "").Trim();
        if (name.Length == 0) return "";
        var sb = new StringBuilder(Math.Min(name.Length, 64));
        foreach (var ch in name)
        {
            if (sb.Length >= 64) break;
            if (char.IsLetterOrDigit(ch) || ch is ' ' or '-' or '_' or '.') sb.Append(ch);
        }
        var result = sb.ToString().Trim().Trim('.');
        while (result.Contains("  ", StringComparison.Ordinal)) result = result.Replace("  ", " ");
        return result is "." or ".." ? "" : result;
    }

    public static string SanitizeFontBase(string? name)
    {
        name = Path.GetFileNameWithoutExtension(name ?? "font");
        var sb = new StringBuilder();
        foreach (var ch in name)
        {
            if (char.IsLetterOrDigit(ch) || ch is '-' or '_') sb.Append(ch);
            if (sb.Length >= 80) break;
        }
        return sb.Length == 0 ? "font" : sb.ToString();
    }

    public static string NormalizeKickChannelSlug(string? value)
    {
        value = (value ?? "").Trim();
        if (value.StartsWith('@')) value = value[1..];
        if (Uri.TryCreate(value, UriKind.Absolute, out var uri) && uri.Host.EndsWith("kick.com", StringComparison.OrdinalIgnoreCase))
            value = uri.AbsolutePath.Trim('/').Split('/', StringSplitOptions.RemoveEmptyEntries).FirstOrDefault() ?? "";
        value = value.Trim().ToLowerInvariant();
        return Regex.IsMatch(value, "^[a-z0-9_]{1,64}$") ? value : "";
    }

    public static async Task AtomicWriteJsonAsync<T>(string path, T value, CancellationToken ct = default)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var bytes = JsonSerializer.SerializeToUtf8Bytes(value, JsonIndented);
        await AtomicWriteAsync(path, bytes, ct);
    }

    public static async Task AtomicWriteAsync(string path, byte[] bytes, CancellationToken ct = default)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var tmp = path + ".tmp-" + Guid.NewGuid().ToString("N");
        await File.WriteAllBytesAsync(tmp, bytes, ct);
        try
        {
            if (File.Exists(path)) File.Replace(tmp, path, null, ignoreMetadataErrors: true);
            else File.Move(tmp, path);
        }
        catch
        {
            if (File.Exists(path)) File.Delete(path);
            File.Move(tmp, path, overwrite: true);
        }
        finally
        {
            try { if (File.Exists(tmp)) File.Delete(tmp); } catch { }
        }
    }

    public static T LoadJsonOrDefault<T>(string path, Func<T> factory, string corruptPrefix) where T : class
    {
        if (!File.Exists(path)) return factory();
        try
        {
            var result = JsonSerializer.Deserialize<T>(File.ReadAllBytes(path), Json);
            return result ?? factory();
        }
        catch
        {
            try
            {
                var stamp = DateTime.Now.ToString("yyyyMMdd-HHmmss");
                File.Copy(path, Path.Combine(Path.GetDirectoryName(path)!, $"{corruptPrefix}.corrupt-{stamp}.json"), overwrite: true);
            }
            catch { }
            return factory();
        }
    }

    public static bool IsLocalHost(string? host)
    {
        host = (host ?? "").Trim();
        var colon = host.LastIndexOf(':');
        if (colon > 0) host = host[..colon];
        host = host.Trim('[', ']');
        return host.Equals("127.0.0.1", StringComparison.OrdinalIgnoreCase) ||
               host.Equals("localhost", StringComparison.OrdinalIgnoreCase) ||
               host.Equals("::1", StringComparison.OrdinalIgnoreCase);
    }

    public static bool MutationOriginAllowed(string? host, string? origin, string? referer)
    {
        if (!IsLocalHost(host)) return false;
        if (!string.IsNullOrWhiteSpace(origin))
        {
            if (!Uri.TryCreate(origin, UriKind.Absolute, out var u)) return false;
            return IsLocalHost(u.IsDefaultPort ? u.Host : $"{u.Host}:{u.Port}");
        }
        if (!string.IsNullOrWhiteSpace(referer))
        {
            if (!Uri.TryCreate(referer, UriKind.Absolute, out var u)) return false;
            return IsLocalHost(u.IsDefaultPort ? u.Host : $"{u.Host}:{u.Port}");
        }
        return true;
    }

    public static string ContentTypeForPath(string path)
    {
        return Path.GetExtension(path).ToLowerInvariant() switch
        {
            ".html" => "text/html; charset=utf-8",
            ".css" => "text/css; charset=utf-8",
            ".js" => "application/javascript; charset=utf-8",
            ".json" => "application/json; charset=utf-8",
            ".png" => "image/png",
            ".jpg" or ".jpeg" => "image/jpeg",
            ".gif" => "image/gif",
            ".webp" => "image/webp",
            ".svg" => "image/svg+xml",
            ".ico" => "image/x-icon",
            ".mp3" => "audio/mpeg",
            ".wav" => "audio/wav",
            ".ogg" => "audio/ogg",
            ".m4a" => "audio/mp4",
            ".mp4" => "video/mp4",
            ".webm" => "video/webm",
            ".ttf" => "font/ttf",
            ".otf" => "font/otf",
            ".woff" => "font/woff",
            ".woff2" => "font/woff2",
            ".zip" => "application/zip",
            ".txt" => "text/plain; charset=utf-8",
            _ => "application/octet-stream"
        };
    }

    public static async Task<byte[]> ReadLimitedAsync(Stream input, long maxBytes, CancellationToken ct)
    {
        using var ms = new MemoryStream();
        var buffer = new byte[64 * 1024];
        long total = 0;
        while (true)
        {
            var read = await input.ReadAsync(buffer, ct);
            if (read <= 0) break;
            total += read;
            if (total > maxBytes) throw new InvalidDataException("Upload is too large.");
            ms.Write(buffer, 0, read);
        }
        return ms.ToArray();
    }

    public static void OpenExternal(string value)
    {
        try { Process.Start(new ProcessStartInfo(value) { UseShellExecute = true }); } catch { }
    }

    public static void OpenFolder(string path)
    {
        Directory.CreateDirectory(path);
        try { Process.Start(new ProcessStartInfo("explorer.exe", path) { UseShellExecute = true }); } catch { }
    }

    public static byte[] ProtectCredential(byte[] plain)
    {
        if (!OperatingSystem.IsWindows()) return plain;
        var inBlob = new DATA_BLOB();
        var outBlob = new DATA_BLOB();
        try
        {
            inBlob.cbData = plain.Length;
            inBlob.pbData = Marshal.AllocHGlobal(plain.Length);
            Marshal.Copy(plain, 0, inBlob.pbData, plain.Length);
            if (!CryptProtectData(ref inBlob, "SleepySource", IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, 0, ref outBlob))
                throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            var result = new byte[outBlob.cbData];
            Marshal.Copy(outBlob.pbData, result, 0, outBlob.cbData);
            return result;
        }
        finally
        {
            if (inBlob.pbData != IntPtr.Zero) Marshal.FreeHGlobal(inBlob.pbData);
            if (outBlob.pbData != IntPtr.Zero) LocalFree(outBlob.pbData);
        }
    }

    public static byte[] UnprotectCredential(byte[] cipher)
    {
        if (!OperatingSystem.IsWindows()) return cipher;
        var inBlob = new DATA_BLOB();
        var outBlob = new DATA_BLOB();
        try
        {
            inBlob.cbData = cipher.Length;
            inBlob.pbData = Marshal.AllocHGlobal(cipher.Length);
            Marshal.Copy(cipher, 0, inBlob.pbData, cipher.Length);
            if (!CryptUnprotectData(ref inBlob, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, 0, ref outBlob))
                throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            var result = new byte[outBlob.cbData];
            Marshal.Copy(outBlob.pbData, result, 0, outBlob.cbData);
            return result;
        }
        finally
        {
            if (inBlob.pbData != IntPtr.Zero) Marshal.FreeHGlobal(inBlob.pbData);
            if (outBlob.pbData != IntPtr.Zero) LocalFree(outBlob.pbData);
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct DATA_BLOB { public int cbData; public IntPtr pbData; }
    [DllImport("crypt32.dll", SetLastError = true, CharSet = CharSet.Unicode)] private static extern bool CryptProtectData(ref DATA_BLOB pDataIn, string? szDataDescr, IntPtr pOptionalEntropy, IntPtr pvReserved, IntPtr pPromptStruct, int dwFlags, ref DATA_BLOB pDataOut);
    [DllImport("crypt32.dll", SetLastError = true)] private static extern bool CryptUnprotectData(ref DATA_BLOB pDataIn, IntPtr ppszDataDescr, IntPtr pOptionalEntropy, IntPtr pvReserved, IntPtr pPromptStruct, int dwFlags, ref DATA_BLOB pDataOut);
    [DllImport("kernel32.dll", SetLastError = true)] private static extern IntPtr LocalFree(IntPtr hMem);
}
