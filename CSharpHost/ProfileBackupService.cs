using System.IO.Compression;
using System.Text.Json;

namespace SleepySource;

internal sealed class ProfileBackupService
{
    private readonly CoreStateService core;
    public ProfileBackupService(CoreStateService core) => this.core = core;

    public async Task SaveProfileAsync(string name)
    {
        name=AppUtil.SanitizeProfileName(name);if(name.Length==0)throw new InvalidDataException("enter a profile name");var dir=Path.Combine(core.ProfileDir,name);Directory.CreateDirectory(dir);RemoveManaged(dir);var s=core.SettingsSnapshot();CoreStateService.NormalizeSettings(s);
        var art=core.CurrentCustomImagePath();if(art!=null){var dst=Path.Combine(dir,"artwork"+Path.GetExtension(art).ToLowerInvariant());File.Copy(art,dst,true);s.CustomImage=Path.GetFileName(dst);}var bg=core.CurrentBackgroundPath();if(bg!=null){var dst=Path.Combine(dir,"background"+Path.GetExtension(bg).ToLowerInvariant());File.Copy(bg,dst,true);s.CustomBackground=Path.GetFileName(dst);}var font=core.FontPathForFamily(s.TextFont);if(font!=null)File.Copy(font,Path.Combine(dir,"font_"+Path.GetFileName(font)),true);await AppUtil.AtomicWriteJsonAsync(Path.Combine(dir,"profile.json"),s);
    }
    public async Task LoadProfileAsync(string name)
    {
        name=AppUtil.SanitizeProfileName(name);if(name.Length==0)throw new InvalidDataException("select a profile");var dir=Path.Combine(core.ProfileDir,name);var path=Path.Combine(dir,"profile.json");if(!File.Exists(path))throw new FileNotFoundException("profile not found");var next=JsonSerializer.Deserialize<Settings>(await File.ReadAllBytesAsync(path),AppUtil.Json)??throw new InvalidDataException("profile is invalid");CoreStateService.NormalizeSettings(next);var global=core.SettingsSnapshot();next.SnapEnabled=global.SnapEnabled;next.GridSize=global.GridSize;next.OnboardingComplete=global.OnboardingComplete;next.DesignerTheme=global.DesignerTheme;next.StartupPage=global.StartupPage;next.LastModule=global.LastModule;next.DefaultProfile=global.DefaultProfile;next.MediaSourceMode=global.MediaSourceMode;next.MediaSourceInclude=global.MediaSourceInclude;next.MediaSourceExclude=global.MediaSourceExclude;
        var art=Directory.EnumerateFiles(dir,"artwork.*").FirstOrDefault();if(art!=null){var target=Path.Combine(core.MediaDir,"custom_now_playing"+Path.GetExtension(art).ToLowerInvariant());DeletePrefix(core.MediaDir,"custom_now_playing",target);File.Copy(art,target,true);next.CustomImage=Path.GetFileName(target);}else next.CustomImage=global.CustomImage;
        var bg=Directory.EnumerateFiles(dir,"background.*").FirstOrDefault();if(bg!=null){var target=Path.Combine(core.MediaDir,"custom_background"+Path.GetExtension(bg).ToLowerInvariant());DeletePrefix(core.MediaDir,"custom_background",target);File.Copy(bg,target,true);next.CustomBackground=Path.GetFileName(target);}else next.CustomBackground=global.CustomBackground;
        var fp=Directory.EnumerateFiles(dir,"font_*").FirstOrDefault();if(fp!=null){var fontName=Path.GetFileName(fp)[5..];var ext=Path.GetExtension(fontName).ToLowerInvariant();if(new[]{".ttf",".otf",".woff",".woff2"}.Contains(ext))File.Copy(fp,Path.Combine(core.FontDir,fontName),true);}if(next.TextFont.StartsWith("NPF_")&&core.FontPathForFamily(next.TextFont)==null)next.TextFont="Segoe UI";await core.ApplySettingsAsync(next);
    }
    public async Task<string> DuplicateProfileAsync(string name,string newName)
    {
        name=AppUtil.SanitizeProfileName(name);newName=AppUtil.SanitizeProfileName(newName);if(name.Length==0)throw new InvalidDataException("select a profile");if(newName.Length==0)newName=UniqueName(name+" Copy");if(name.Equals(newName,StringComparison.OrdinalIgnoreCase))throw new InvalidDataException("choose a different name for the duplicate");CopyProfileDirectory(Path.Combine(core.ProfileDir,name),Path.Combine(core.ProfileDir,newName));await Task.CompletedTask;return newName;
    }
    public async Task RenameProfileAsync(string name,string newName)
    {
        name=AppUtil.SanitizeProfileName(name);newName=AppUtil.SanitizeProfileName(newName);if(name.Length==0||newName.Length==0)throw new InvalidDataException("select a profile and enter its new name");var src=Path.Combine(core.ProfileDir,name);var dst=Path.Combine(core.ProfileDir,newName);if(!Directory.Exists(src))throw new FileNotFoundException("profile not found");if(!name.Equals(newName,StringComparison.OrdinalIgnoreCase)&&Directory.Exists(dst))throw new InvalidDataException("a profile with that name already exists");if(name.Equals(newName,StringComparison.OrdinalIgnoreCase)&&name!=newName){var tmp=Path.Combine(core.ProfileDir,".rename-"+Guid.NewGuid().ToString("N"));Directory.Move(src,tmp);Directory.Move(tmp,dst);}else if(src!=dst)Directory.Move(src,dst);var s=core.SettingsSnapshot();if(s.DefaultProfile.Equals(name,StringComparison.OrdinalIgnoreCase)){s.DefaultProfile=newName;await core.ApplySettingsAsync(s);}
    }
    public async Task SetDefaultAsync(string name)
    {
        name=AppUtil.SanitizeProfileName(name);if(name.Length>0&&!File.Exists(Path.Combine(core.ProfileDir,name,"profile.json")))throw new FileNotFoundException("profile not found");var s=core.SettingsSnapshot();s.DefaultProfile=name;await core.ApplySettingsAsync(s);
    }
    public async Task DeleteProfileAsync(string name){name=AppUtil.SanitizeProfileName(name);if(name.Length==0)throw new InvalidDataException("select a profile");Directory.Delete(Path.Combine(core.ProfileDir,name),true);var s=core.SettingsSnapshot();if(s.DefaultProfile.Equals(name,StringComparison.OrdinalIgnoreCase)){s.DefaultProfile="";await core.ApplySettingsAsync(s);}}
    public async Task LoadDefaultAtStartupAsync(){var name=core.SettingsSnapshot().DefaultProfile;if(name.Length==0)return;if(!File.Exists(Path.Combine(core.ProfileDir,name,"profile.json"))){await SetDefaultAsync("");return;}await LoadProfileAsync(name);}

    public byte[] ExportProfile(string name)
    {
        name=AppUtil.SanitizeProfileName(name);if(name.Length==0)throw new InvalidDataException("select a profile to export");var dir=Path.Combine(core.ProfileDir,name);if(!File.Exists(Path.Combine(dir,"profile.json")))throw new FileNotFoundException("profile not found");using var ms=new MemoryStream();using(var zip=new ZipArchive(ms,ZipArchiveMode.Create,true)){var man=zip.CreateEntry("bundle.json");using(var s=man.Open())JsonSerializer.Serialize(s,new{format="SleepySourceProfileBundle",version=1,name},AppUtil.JsonIndented);foreach(var file in Directory.EnumerateFiles(dir)){var n=Path.GetFileName(file);if(!AllowedBundleEntry(n)||n.Equals("bundle.json",StringComparison.OrdinalIgnoreCase))continue;zip.CreateEntryFromFile(file,n,CompressionLevel.Optimal);}}return ms.ToArray();
    }
    public async Task<string> ImportProfileAsync(byte[] zipBytes,string uploadedName,bool overwrite)
    {
        if(zipBytes.LongLength>AppUtil.MaxProfileBundleBytes)throw new InvalidDataException("profile bundle is too large");using var ms=new MemoryStream(zipBytes);using var zip=new ZipArchive(ms,ZipArchiveMode.Read);if(zip.Entries.Count is 0 or >32)throw new InvalidDataException("profile bundle has an invalid file count");long total=0;string name="";foreach(var e in zip.Entries){if(e.FullName!=Path.GetFileName(e.FullName)||!AllowedBundleEntry(e.FullName))throw new InvalidDataException("profile bundle contains unsupported files");total+=e.Length;if(total>AppUtil.MaxProfileBundleBytes)throw new InvalidDataException("profile bundle expands beyond the allowed size");if(e.Name.Equals("bundle.json",StringComparison.OrdinalIgnoreCase)){using var doc=JsonDocument.Parse(e.Open());if(doc.RootElement.TryGetProperty("name",out var n))name=AppUtil.SanitizeProfileName(n.GetString());}}
        if(name.Length==0){var stem=Path.GetFileNameWithoutExtension(uploadedName).Replace(".sleepyprofile","").Replace('_',' ');name=AppUtil.SanitizeProfileName(stem);}if(name.Length==0)name="Imported Profile";var final=Path.Combine(core.ProfileDir,name);if(Directory.Exists(final)&&!overwrite)name=UniqueName(name);final=Path.Combine(core.ProfileDir,name);var stage=Path.Combine(core.ProfileDir,".import-"+Guid.NewGuid().ToString("N"));Directory.CreateDirectory(stage);try{foreach(var e in zip.Entries){if(e.Name.Equals("bundle.json",StringComparison.OrdinalIgnoreCase))continue;var dst=Path.Combine(stage,e.Name);await using var input=e.Open();await using var output=File.Create(dst);await input.CopyToAsync(output);}var profile=Path.Combine(stage,"profile.json");if(!File.Exists(profile))throw new InvalidDataException("profile bundle is missing profile.json");var parsed=JsonSerializer.Deserialize<Settings>(await File.ReadAllBytesAsync(profile),AppUtil.Json)??throw new InvalidDataException("profile bundle contains invalid settings");CoreStateService.NormalizeSettings(parsed);if(overwrite&&Directory.Exists(final))Directory.Delete(final,true);Directory.CreateDirectory(final);foreach(var f in Directory.EnumerateFiles(stage))File.Copy(f,Path.Combine(final,Path.GetFileName(f)),true);}finally{if(Directory.Exists(stage))Directory.Delete(stage,true);}return name;
    }

    public byte[] ExportBackup()
    {
        using var ms=new MemoryStream();using(var zip=new ZipArchive(ms,ZipArchiveMode.Create,true)){var m=zip.CreateEntry("SleepySource_Backup.json");using(var s=m.Open())JsonSerializer.Serialize(s,new{product="SleepySource",version=AppUtil.DisplayVersion,created_at=DateTime.UtcNow.ToString("O")},AppUtil.JsonIndented);foreach(var file in Directory.EnumerateFiles(core.DataDir,"*",SearchOption.AllDirectories)){var rel=Path.GetRelativePath(core.DataDir,file);if(SkipBackup(rel))continue;zip.CreateEntryFromFile(file,"SleepySource_Data/"+rel.Replace('\\','/'),CompressionLevel.Optimal);}}return ms.ToArray();
    }

    public async Task RestoreBackupAsync(byte[] zipBytes, ChatService chat, CountdownService countdown)
    {
        if (zipBytes.LongLength > AppUtil.MaxBackupUploadBytes) throw new InvalidDataException("backup file is too large");
        var restore = Path.Combine(core.ExeDir, ".SleepySource_Restore-" + Guid.NewGuid().ToString("N"));
        var rollback = Path.Combine(core.ExeDir, ".SleepySource_Rollback-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(restore); Directory.CreateDirectory(rollback);
        try
        {
            using var ms = new MemoryStream(zipBytes); using var zip = new ZipArchive(ms, ZipArchiveMode.Read);
            if (zip.Entries.Count is 0 or > AppUtil.MaxBackupFiles) throw new InvalidDataException("backup contains an invalid number of files");
            long total = 0; var found = false;
            foreach (var e in zip.Entries)
            {
                var raw = e.FullName.Replace('\\','/').TrimStart('/');
                if (raw.Contains("../", StringComparison.Ordinal) || raw.StartsWith("..", StringComparison.Ordinal) || Path.IsPathRooted(raw)) throw new InvalidDataException("backup contains an unsafe file path");
                var parts = raw.Split('/', StringSplitOptions.RemoveEmptyEntries);
                if (parts.Length == 1 && parts[0].Equals("SleepySource_Backup.json", StringComparison.OrdinalIgnoreCase)) continue;
                if (parts.Length < 2 || !parts[0].Equals("SleepySource_Data", StringComparison.OrdinalIgnoreCase)) continue;
                var rel = Path.Combine(parts.Skip(1).ToArray()); if (SkipBackup(rel)) continue; found = true;
                if (e.Length > AppUtil.MaxBackupExtractBytes) throw new InvalidDataException("backup contains an oversized file");
                total += e.Length; if (total > AppUtil.MaxBackupExtractBytes) throw new InvalidDataException("backup expands beyond the allowed size");
                if (string.IsNullOrEmpty(e.Name)) { Directory.CreateDirectory(Path.Combine(restore, rel)); continue; }
                var target = Path.GetFullPath(Path.Combine(restore, rel)); if (!target.StartsWith(Path.GetFullPath(restore) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException("backup contains an unsafe file path");
                Directory.CreateDirectory(Path.GetDirectoryName(target)!); await using var input=e.Open(); await using var output=File.Create(target); await input.CopyToAsync(output);
            }
            if (!found) throw new InvalidDataException("backup does not contain SleepySource_Data");
            if (!File.Exists(Path.Combine(restore,"settings.json"))) throw new InvalidDataException("backup does not contain SleepySource settings");
            CopyRestorable(core.DataDir, rollback);
            try
            {
                ClearRestorable(core.DataDir); CopyTree(restore, core.DataDir);
                core.ReloadFromDisk(); chat.ReloadFromDisk(); countdown.ReloadFromDisk();
            }
            catch
            {
                ClearRestorable(core.DataDir); CopyTree(rollback, core.DataDir);
                core.ReloadFromDisk(); chat.ReloadFromDisk(); countdown.ReloadFromDisk();
                throw new IOException("backup restore failed; the previous setup was restored");
            }
        }
        finally { try{Directory.Delete(restore,true);}catch{} try{Directory.Delete(rollback,true);}catch{} }
    }

    private static void CopyRestorable(string src,string dst){if(!Directory.Exists(src))return;foreach(var f in Directory.EnumerateFiles(src,"*",SearchOption.AllDirectories)){var rel=Path.GetRelativePath(src,f);if(PreserveRestore(rel)||SkipBackup(rel))continue;var t=Path.Combine(dst,rel);Directory.CreateDirectory(Path.GetDirectoryName(t)!);File.Copy(f,t,true);}}
    private static void ClearRestorable(string dir){if(!Directory.Exists(dir))return;foreach(var f in Directory.EnumerateFiles(dir,"*",SearchOption.AllDirectories).OrderByDescending(x=>x.Length)){var rel=Path.GetRelativePath(dir,f);if(PreserveRestore(rel))continue;try{File.Delete(f);}catch{}}foreach(var d in Directory.EnumerateDirectories(dir,"*",SearchOption.AllDirectories).OrderByDescending(x=>x.Length)){var rel=Path.GetRelativePath(dir,d);if(PreserveRestore(rel))continue;try{if(!Directory.EnumerateFileSystemEntries(d).Any())Directory.Delete(d);}catch{}}}
    private static void CopyTree(string src,string dst){if(!Directory.Exists(src))return;foreach(var f in Directory.EnumerateFiles(src,"*",SearchOption.AllDirectories)){var rel=Path.GetRelativePath(src,f);var t=Path.Combine(dst,rel);Directory.CreateDirectory(Path.GetDirectoryName(t)!);File.Copy(f,t,true);}}

    public static bool SkipBackup(string rel){var clean=rel.Replace('\\','/').ToLowerInvariant();if(clean=="alerts"||clean.StartsWith("alerts/")||clean=="kick"||clean.StartsWith("kick/")||clean=="desktopruntime"||clean.StartsWith("desktopruntime/")||clean=="app.ico"||clean=="kick_connection.json"||clean=="kick_credentials.json"||clean=="kick_user_authorization.json")return true;var b=Path.GetFileName(clean);return b.EndsWith(".tmp")||new[]{".sleepysource-settings-",".chat-settings-",".kick-credentials-",".kick-user-auth-",".font-upload-",".countdown-font-upload-",".chat-font-upload-",".artwork-upload-",".background-upload-",".icon-",".profile-copy-",".bundle-"}.Any(prefix=>b.StartsWith(prefix,StringComparison.Ordinal));}
    public static bool PreserveRestore(string rel){var c=rel.Replace('\\','/').ToLowerInvariant();return c=="desktopruntime"||c.StartsWith("desktopruntime/")||c=="app.ico"||c=="kick_connection.json"||c=="kick_credentials.json"||c=="kick_user_authorization.json";}

    private string UniqueName(string baseName){var existing=core.ListProfiles().Select(x=>x.Name).ToHashSet(StringComparer.OrdinalIgnoreCase);baseName=AppUtil.SanitizeProfileName(baseName);if(!existing.Contains(baseName))return baseName;for(var i=2;i<=99;i++){var n=AppUtil.SanitizeProfileName($"{baseName} Copy {i}");if(!existing.Contains(n))return n;}return AppUtil.SanitizeProfileName(baseName+" Imported");}
    private static void CopyProfileDirectory(string src,string dst){if(!File.Exists(Path.Combine(src,"profile.json")))throw new FileNotFoundException("profile not found");if(Directory.Exists(dst))throw new InvalidDataException("a profile with that name already exists");Directory.CreateDirectory(dst);try{foreach(var f in Directory.EnumerateFiles(src)){var n=Path.GetFileName(f);if(AllowedBundleEntry(n)&&!n.Equals("bundle.json",StringComparison.OrdinalIgnoreCase))File.Copy(f,Path.Combine(dst,n));}}catch{Directory.Delete(dst,true);throw;}}
    private static bool AllowedBundleEntry(string n){var b=Path.GetFileName(n);if(b!=n||b.Length==0)return false;var l=b.ToLowerInvariant();if(l is "bundle.json" or "profile.json")return true;var ext=Path.GetExtension(l);if(l.StartsWith("artwork.")||l.StartsWith("background."))return new[]{".png",".jpg",".jpeg",".gif",".webp",".webm"}.Contains(ext);if(l.StartsWith("font_"))return new[]{".ttf",".otf",".woff",".woff2"}.Contains(ext);return false;}
    private static void RemoveManaged(string dir){foreach(var f in Directory.EnumerateFiles(dir)){var n=Path.GetFileName(f).ToLowerInvariant();if(n is "profile.json" or "bundle.json"||n.StartsWith("artwork.")||n.StartsWith("background.")||n.StartsWith("font_"))File.Delete(f);}}
    private static void DeletePrefix(string dir,string prefix,string except){foreach(var f in Directory.EnumerateFiles(dir,prefix+".*"))if(!Path.GetFullPath(f).Equals(Path.GetFullPath(except),StringComparison.OrdinalIgnoreCase))try{File.Delete(f);}catch{}}
}
