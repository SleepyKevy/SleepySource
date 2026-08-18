using System.Net;
using System.Text.Json;

namespace SleepySource;

internal sealed class KickAssetsService : IDisposable
{
    private readonly ChatService chat;
    private readonly HttpClient http;
    private static readonly Dictionary<string,string> RoleBadges = new(StringComparer.OrdinalIgnoreCase)
    {
        ["broadcaster"]="broadcaster.svg",["moderator"]="moderator.svg",["vip"]="vip.svg",["og"]="og.svg",["founder"]="founder.svg",["subscriber"]="subscriber.svg",["verified"]="verified.svg",["staff"]="staff.svg",["sidekick"]="sidekick.svg",["sub_gifter"]="subGifter.svg",["sub_gifter_25"]="subGifter25.svg",["sub_gifter_50"]="subGifter50.svg",["sub_gifter_100"]="subGifter100.svg",["sub_gifter_200"]="subGifter200.svg"
    };

    public KickAssetsService(ChatService chat)
    {
        this.chat=chat;
        http=new HttpClient(new HttpClientHandler{AllowAutoRedirect=false}){Timeout=TimeSpan.FromSeconds(8)};
        http.DefaultRequestHeaders.UserAgent.ParseAdd("SleepySource/1.0");
    }

    public async Task<object> SevenTVAsync(string? setId,string? kickChannel,CancellationToken ct)
    {
        var state=chat.State();
        if(!state.Settings.SevenTVEnabled)return new{enabled=false,channel=new{emotes=Array.Empty<object>()},global=new{emotes=Array.Empty<object>()}};
        setId=(setId??"").Trim();if(setId.Length==0)setId=state.Settings.SevenTVEmoteSetID.Trim();
        string endpoint;
        if(setId.Length>0)endpoint="https://7tv.io/v3/emote-sets/"+Uri.EscapeDataString(setId);
        else
        {
            var channel=AppUtil.NormalizeKickChannelSlug(kickChannel);if(channel.Length==0)channel=state.ConnectedChannel;
            var userId=state.BroadcasterUserID;
            if(userId.Length==0)
                return new{enabled=true,channel=new{emotes=Array.Empty<object>()},global=new{emotes=Array.Empty<object>()},warning="Connect with Kick to resolve the channel's 7TV emotes automatically."};
            endpoint="https://7tv.io/v3/users/kick/"+Uri.EscapeDataString(userId);
        }
        var channelJson=await FetchJsonAsync(endpoint,4L<<20,ct);
        JsonElement global;
        try{global=await FetchJsonAsync("https://7tv.io/v3/emote-sets/global",4L<<20,ct);}catch{using var d=JsonDocument.Parse("{\"emotes\":[]}");global=d.RootElement.Clone();}
        return new{enabled=true,channel=channelJson,global};
    }

    public Task<(byte[] Data,string ContentType)> SevenTVImageAsync(string id,CancellationToken ct)
    {
        id=(id??"").Trim();if(id.Length<3||id.Length>80||id.Any(ch=>!(char.IsLetterOrDigit(ch)||ch=='_'||ch=='-')))throw new InvalidDataException("invalid 7TV emote ID");
        return ProxyImageAsync("https://cdn.7tv.app/emote/"+Uri.EscapeDataString(id)+"/2x.webp",false,["cdn.7tv.app"],8L<<20,ct);
    }
    public Task<(byte[] Data,string ContentType)> KickEmoteAsync(string id,CancellationToken ct){id=(id??"").Trim();if(id.Length is <1 or >20||id.Any(ch=>!char.IsDigit(ch)))throw new InvalidDataException("invalid Kick emote ID");return ProxyImageAsync("https://files.kick.com/emotes/"+Uri.EscapeDataString(id)+"/fullsize",true,[],2L<<20,ct);}
    public Task<(byte[] Data,string ContentType)> AvatarAsync(string url,CancellationToken ct){if(!TrustedKick(url))throw new InvalidDataException("untrusted Kick avatar URL");return ProxyImageAsync(url,true,[],2L<<20,ct);}

    public async Task<object> BadgeCatalogAsync(string? channel,CancellationToken ct)
    {
        channel=AppUtil.NormalizeKickChannelSlug(channel);if(channel.Length==0)channel=chat.State().Settings.KickChannel;
        try
        {
            var data=await FetchBytesAsync("https://kick.com/api/v2/channels/"+Uri.EscapeDataString(channel),2L<<20,ct);
            using var doc=JsonDocument.Parse(data);var list=new List<(int Months,string URL)>();
            if(doc.RootElement.TryGetProperty("subscriber_badges",out var badges)&&badges.ValueKind==JsonValueKind.Array)
                foreach(var b in badges.EnumerateArray()){var months=b.TryGetProperty("months",out var m)&&m.TryGetInt32(out var n)?n:0;var u="";if(b.TryGetProperty("badge_image",out var bi)&&bi.TryGetProperty("src",out var src))u=src.GetString()?.Trim()??"";if(months>0&&TrustedKick(u))list.Add((months,u));}
            return new{channel,subscriber_badges=list.OrderBy(x=>x.Months).Select(x=>new{months=x.Months,url=x.URL}).ToList()};
        }
        catch(Exception ex){return new{channel,subscriber_badges=Array.Empty<object>(),warning=ex.Message};}
    }

    public async Task<(byte[] Data,string ContentType)> BadgeImageAsync(string? rawUrl,string? role,int count,CancellationToken ct)
    {
        if(!string.IsNullOrWhiteSpace(rawUrl))return await ProxyImageAsync(rawUrl,true,[],2L<<20,ct);
        var normalized=ChatService.NormalizeBadgeType(role);if(normalized=="sub_gifter")normalized=count>=200?"sub_gifter_200":count>=100?"sub_gifter_100":count>=50?"sub_gifter_50":count>=25?"sub_gifter_25":"sub_gifter";
        if(!RoleBadges.TryGetValue(normalized,out var file))throw new FileNotFoundException("unknown Kick badge");
        foreach(var url in new[]{"https://www.kickdatabase.com/kickBadges/"+file,"https://cpwemotes.co.uk/kick/kickBadges/"+file})
            try{return await ProxyImageAsync(url,false,["www.kickdatabase.com","cpwemotes.co.uk"],2L<<20,ct);}catch{}
        throw new IOException("Kick badge art unavailable");
    }

    private async Task<JsonElement> FetchJsonAsync(string url,long max,CancellationToken ct){var bytes=await FetchBytesAsync(url,max,ct);using var d=JsonDocument.Parse(bytes);return d.RootElement.Clone();}
    private async Task<byte[]> FetchBytesAsync(string url,long max,CancellationToken ct){using var resp=await http.GetAsync(url,HttpCompletionOption.ResponseHeadersRead,ct);if(!resp.IsSuccessStatusCode)throw new HttpRequestException($"request returned HTTP {(int)resp.StatusCode}");await using var s=await resp.Content.ReadAsStreamAsync(ct);return await ReadLimitedAsync(s,max,ct);}
    private async Task<(byte[] Data,string ContentType)> ProxyImageAsync(string url,bool trustedKickOnly,string[] allowed,long max,CancellationToken ct)
    {
        if(trustedKickOnly?!TrustedKick(url):!AllowedURL(url,allowed))throw new InvalidDataException("untrusted image URL");
        var current=url;
        for(var redirects=0;redirects<4;redirects++)
        {
            using var resp=await http.GetAsync(current,HttpCompletionOption.ResponseHeadersRead,ct);
            if((int)resp.StatusCode is >=300 and <400 && resp.Headers.Location is not null){var next=resp.Headers.Location.IsAbsoluteUri?resp.Headers.Location:new Uri(new Uri(current),resp.Headers.Location);current=next.ToString();if(trustedKickOnly?!TrustedKick(current):!AllowedURL(current,allowed))throw new InvalidDataException("untrusted redirect");continue;}
            if(!resp.IsSuccessStatusCode)throw new HttpRequestException("image returned "+resp.StatusCode);
            var type=(resp.Content.Headers.ContentType?.MediaType??"").ToLowerInvariant();if(!type.StartsWith("image/"))throw new InvalidDataException("invalid image response");await using var s=await resp.Content.ReadAsStreamAsync(ct);return(await ReadLimitedAsync(s,max,ct),type);
        }
        throw new HttpRequestException("too many redirects");
    }
    private static bool AllowedURL(string raw,string[] allowed){if(!Uri.TryCreate(raw,UriKind.Absolute,out var u)||u.Scheme!="https"||u.UserInfo.Length>0)return false;return allowed.Length==0||allowed.Contains(u.Host,StringComparer.OrdinalIgnoreCase);}
    private static bool TrustedKick(string raw){if(!Uri.TryCreate(raw,UriKind.Absolute,out var u)||u.Scheme!="https"||u.UserInfo.Length>0)return false;var h=u.Host.TrimEnd('.').ToLowerInvariant();return h=="kick.com"||h.EndsWith(".kick.com",StringComparison.Ordinal);}
    private static async Task<byte[]> ReadLimitedAsync(Stream s,long max,CancellationToken ct){using var ms=new MemoryStream();var b=new byte[81920];long total=0;while(true){var n=await s.ReadAsync(b,ct);if(n==0)break;total+=n;if(total>max)throw new InvalidDataException("response is too large");ms.Write(b,0,n);}return ms.ToArray();}
    public void Dispose()=>http.Dispose();
}
