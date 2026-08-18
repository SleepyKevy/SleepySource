using System.Text.Json;
using System.Text.RegularExpressions;

namespace SleepySource;

internal sealed class UpdateService : IDisposable
{
    public const string RepositoryURL = "https://github.com/SleepyKevy/SleepySource";
    private const string API = "https://api.github.com/repos/SleepyKevy/SleepySource/releases/latest";
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(6) };
    private static readonly Regex Numbers = new(@"\d+", RegexOptions.Compiled);

    public UpdateService()
    {
        http.DefaultRequestHeaders.UserAgent.ParseAdd("SleepySource/1.0");
        http.DefaultRequestHeaders.Accept.ParseAdd("application/vnd.github+json");
    }

    public async Task<object> CheckAsync(CancellationToken ct)
    {
        var checkedAt=DateTime.UtcNow.ToString("O");
        try
        {
            using var resp=await http.GetAsync(API,ct);
            if(!resp.IsSuccessStatusCode) return new{status="error",current_version=AppUtil.DisplayVersion,checked_at=checkedAt,update_available=false,message=resp.StatusCode==System.Net.HttpStatusCode.NotFound?"No published SleepySource release was found on GitHub.":"Unable to check for updates."};
            using var doc=JsonDocument.Parse(await resp.Content.ReadAsByteArrayAsync(ct));
            var r=doc.RootElement;
            var tag=Get(r,"tag_name"); var name=Get(r,"name"); var latest=(tag.Length>0?tag:name).TrimStart('v','V');
            var cmp=Compare("1.0.0",latest);
            var notes=Get(r,"body").Replace("\r\n","\n").Trim(); if(notes.Length>12000)notes=notes[..12000]+"\n\n…View the full release notes on GitHub.";
            var url=Get(r,"html_url");if(url.Length==0)url=RepositoryURL+"/releases/latest";
            var status=cmp<0?"available":cmp>0?"ahead":"up_to_date";
            return new{status,current_version=AppUtil.DisplayVersion,latest_version=latest,release_name=name,release_url=url,release_notes=notes,published_at=Get(r,"published_at"),checked_at=checkedAt,update_available=cmp<0,message=cmp<0?$"SleepySource {latest} is available.":cmp>0?"This SleepySource build is newer than the latest published stable release.":"SleepySource is up to date."};
        }
        catch{return new{status="error",current_version=AppUtil.DisplayVersion,checked_at=checkedAt,update_available=false,message="Unable to check for updates."};}
    }

    private static string Get(JsonElement r,string p)=>r.TryGetProperty(p,out var x)?x.GetString()?.Trim()??"":"";
    private static int Compare(string a,string b){var l=Parts(a);var r=Parts(b);if(l.Count==0||r.Count==0)throw new InvalidDataException();for(int i=0;i<Math.Max(l.Count,r.Count);i++){var x=i<l.Count?l[i]:0;var y=i<r.Count?r[i]:0;if(x!=y)return x.CompareTo(y);}return 0;}
    private static List<int> Parts(string s)=>Numbers.Matches(s).Select(m=>int.Parse(m.Value)).ToList();
    public void Dispose()=>http.Dispose();
}
