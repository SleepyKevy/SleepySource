using System.Net;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace SleepySource;

internal sealed record KickToken(string Token, DateTime ExpiresAt);
internal sealed record KickChannelMeta(long BroadcasterUserID, string ChannelSlug, string Title, long CategoryID, string CategoryName, bool IsLive);
internal sealed record KickCategory(long ID, string Name);

internal sealed class KickService : IDisposable
{
    public const string APIBase = "https://api.kick.com/public/v1";
    public const string CategoriesAPIBase = "https://api.kick.com/public/v2";
    public const string OAuthBase = "https://id.kick.com";
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(15) };
    private readonly object keyGate = new();
    private RSA? webhookKey;
    private DateTime webhookKeyAt;

    public KickService()
    {
        http.DefaultRequestHeaders.UserAgent.ParseAdd("SleepySource/1.0");
        http.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
    }

    public async Task<KickToken> RequestAppAccessTokenAsync(string clientID, string clientSecret, CancellationToken ct)
    {
        clientID = clientID.Trim(); clientSecret = clientSecret.Trim();
        if (clientID.Length == 0 || clientSecret.Length == 0) throw new InvalidOperationException("enter your Kick Client ID and Client Secret first");
        using var request = new HttpRequestMessage(HttpMethod.Post, OAuthBase + "/oauth/token")
        {
            Content = new FormUrlEncodedContent(new Dictionary<string,string> { ["grant_type"]="client_credentials", ["client_id"]=clientID, ["client_secret"]=clientSecret })
        };
        using var response = await http.SendAsync(request, ct); var data = await response.Content.ReadAsByteArrayAsync(ct);
        if (!response.IsSuccessStatusCode)
        {
            if (response.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden) throw new InvalidOperationException("Kick rejected the Client ID or Client Secret");
            throw new InvalidOperationException("Kick OAuth: " + ExtractError(data, response.ReasonPhrase ?? "request failed"));
        }
        using var doc = JsonDocument.Parse(data); var root = doc.RootElement; var token = root.TryGetProperty("access_token", out var at) ? at.GetString()?.Trim() ?? "" : ""; if (token.Length == 0) throw new InvalidOperationException("Kick OAuth did not return an access token");
        var seconds = 3600L; if (root.TryGetProperty("expires_in", out var ex)) { if (ex.ValueKind == JsonValueKind.Number) ex.TryGetInt64(out seconds); else long.TryParse(ex.GetString(), out seconds); } seconds = Math.Max(60, seconds);
        return new KickToken(token, DateTime.UtcNow.AddSeconds(seconds));
    }

    public async Task<(string UserID, string Slug)> ResolveBroadcasterAsync(string channel, string token, CancellationToken ct)
    {
        channel = AppUtil.NormalizeKickChannelSlug(channel); if (channel.Length == 0) throw new InvalidOperationException("Kick channel username required"); if (string.IsNullOrWhiteSpace(token)) throw new InvalidOperationException("connect Kick with a Client ID and Client Secret first");
        var data = await APIAsync(HttpMethod.Get, APIBase + "/channels?slug=" + Uri.EscapeDataString(channel), token, null, ct);
        using var doc = JsonDocument.Parse(data); if (!doc.RootElement.TryGetProperty("data", out var arr) || arr.ValueKind != JsonValueKind.Array || arr.GetArrayLength() == 0) throw new InvalidOperationException($"Kick channel \"{channel}\" was not found");
        JsonElement item = arr[0]; foreach (var x in arr.EnumerateArray()) if (x.TryGetProperty("slug", out var sl) && string.Equals(sl.GetString()?.Trim(), channel, StringComparison.OrdinalIgnoreCase)) { item = x; break; }
        var id = item.TryGetProperty("broadcaster_user_id", out var bi) && bi.TryGetInt64(out var n) ? n : 0; if (id <= 0) throw new InvalidOperationException($"Kick did not return a broadcaster user ID for \"{channel}\"");
        var slug = AppUtil.NormalizeKickChannelSlug(item.TryGetProperty("slug", out var s) ? s.GetString() : channel); if (slug.Length == 0) slug = channel; return (id.ToString(), slug);
    }

    public async Task<(string SubscriptionID, int Replaced)> RefreshSubscriptionsAsync(string token, string broadcasterUserID, CancellationToken ct)
    {
        if (!long.TryParse(broadcasterUserID.Trim(), out var id) || id <= 0) throw new InvalidOperationException("invalid Kick broadcaster user ID");
        string[] desired = ["chat.message.sent","channel.followed","channel.subscription.new","channel.subscription.renewal","channel.subscription.gifts","kicks.gifted","channel.reward.redemption.updated"];
        var listBytes = await APIAsync(HttpMethod.Get, APIBase + "/events/subscriptions?broadcaster_user_id=" + id, token, null, ct);
        var ids = new List<string>(); using (var doc = JsonDocument.Parse(listBytes)) if (doc.RootElement.TryGetProperty("data", out var arr) && arr.ValueKind == JsonValueKind.Array) foreach (var x in arr.EnumerateArray())
        {
            var ev = x.TryGetProperty("event", out var e) ? e.GetString()?.Trim() ?? "" : ""; var ver = x.TryGetProperty("version", out var v) && v.TryGetInt32(out var vi) ? vi : 0; var sid = x.TryGetProperty("id", out var si) ? si.GetString()?.Trim() ?? "" : ""; if (ver == 1 && sid.Length > 0 && desired.Contains(ev, StringComparer.OrdinalIgnoreCase)) ids.Add(sid);
        }
        if (ids.Count > 0)
        {
            var q = string.Join("&", ids.Select(x => "id=" + Uri.EscapeDataString(x))); await APIAsync(HttpMethod.Delete, APIBase + "/events/subscriptions?" + q, token, null, ct);
        }
        var payload = new { broadcaster_user_id = id, events = desired.Select(x => new { name=x, version=1 }).ToArray(), method="webhook" };
        var createdBytes = await APIAsync(HttpMethod.Post, APIBase + "/events/subscriptions", token, JsonSerializer.SerializeToUtf8Bytes(payload, AppUtil.Json), ct);
        var confirmed = new Dictionary<string,string>(StringComparer.OrdinalIgnoreCase); using (var doc = JsonDocument.Parse(createdBytes))
        {
            if (!doc.RootElement.TryGetProperty("data", out var arr) || arr.ValueKind != JsonValueKind.Array) throw new InvalidOperationException("Kick returned an invalid event subscription response");
            foreach (var x in arr.EnumerateArray())
            {
                var name = x.TryGetProperty("name", out var nm) ? nm.GetString()?.Trim() ?? "" : ""; var ver = x.TryGetProperty("version", out var vv) && vv.TryGetInt32(out var vi) ? vi : 0; if (!desired.Contains(name, StringComparer.OrdinalIgnoreCase) || ver != 1) continue;
                var err = x.TryGetProperty("error", out var er) ? er.GetString()?.Trim() ?? "" : ""; if (err.Length > 0) throw new InvalidOperationException($"Kick could not subscribe to {name}: {err}");
                var sid = x.TryGetProperty("subscription_id", out var si) ? si.GetString()?.Trim() ?? "" : ""; if (sid.Length == 0) throw new InvalidOperationException($"Kick did not return a subscription ID for {name}"); confirmed[name] = sid;
            }
        }
        foreach (var d in desired) if (!confirmed.ContainsKey(d)) throw new InvalidOperationException($"Kick did not confirm the {d} subscription");
        return (confirmed["chat.message.sent"], ids.Count);
    }

    public async Task<KickChannelMeta> FetchChannelMetadataAsync(string slug, string token, CancellationToken ct)
    {
        slug = AppUtil.NormalizeKickChannelSlug(slug); if (slug.Length == 0) throw new InvalidOperationException("Kick channel not connected");
        var data = await APIAsync(HttpMethod.Get, APIBase + "/channels?slug=" + Uri.EscapeDataString(slug), token, null, ct); return ParseChannelMetadata(data, slug);
    }
    public async Task<KickChannelMeta> FetchAuthorizedChannelMetadataAsync(string token, CancellationToken ct) => ParseChannelMetadata(await APIAsync(HttpMethod.Get, APIBase + "/channels", token, null, ct), "");
    public async Task<(KickChannelMeta Meta, bool Live)> FetchActiveLivestreamAsync(long id, string token, CancellationToken ct)
    {
        if (id <= 0) throw new InvalidOperationException("invalid Kick broadcaster user ID"); var data = await APIAsync(HttpMethod.Get, APIBase + "/users/livestreams?user_id=" + id, token, null, ct); using var doc = JsonDocument.Parse(data); if (!doc.RootElement.TryGetProperty("data", out var arr) || arr.ValueKind != JsonValueKind.Array || arr.GetArrayLength() == 0) return (new KickChannelMeta(id,"","",0,"",false), false);
        var x = arr[0]; long user=id; if (x.TryGetProperty("broadcaster_user", out var bu) && bu.TryGetProperty("id", out var ui) && ui.TryGetInt64(out var n) && n>0) user=n; var cat=x.TryGetProperty("category", out var ca)?ca:default; var channel=x.TryGetProperty("channel", out var ch)?ch:default;
        return (new KickChannelMeta(user, channel.ValueKind==JsonValueKind.Object&&channel.TryGetProperty("slug",out var sl)?AppUtil.NormalizeKickChannelSlug(sl.GetString()):"", x.TryGetProperty("title",out var ti)?ti.GetString()?.Trim()??"":"", cat.ValueKind==JsonValueKind.Object&&cat.TryGetProperty("id",out var ci)&&ci.TryGetInt64(out var cid)?cid:0, cat.ValueKind==JsonValueKind.Object&&cat.TryGetProperty("name",out var cn)?cn.GetString()?.Trim()??"":"", true), true);
    }
    public async Task<List<KickCategory>> SearchCategoriesAsync(string query, string token, CancellationToken ct)
    {
        query=query.Trim(); if(query.Length==0)return []; var data=await APIAsync(HttpMethod.Get, CategoriesAPIBase+"/categories?limit=20&name="+Uri.EscapeDataString(query),token,null,ct); var list=new List<KickCategory>(); var seen=new HashSet<long>(); using var doc=JsonDocument.Parse(data); if(doc.RootElement.TryGetProperty("data",out var arr)&&arr.ValueKind==JsonValueKind.Array)foreach(var x in arr.EnumerateArray()){var id=x.TryGetProperty("id",out var i)&&i.TryGetInt64(out var n)?n:0;var name=x.TryGetProperty("name",out var nm)?nm.GetString()?.Trim()??"":"";if(id>0&&name.Length>0&&seen.Add(id))list.Add(new(id,name));} return list;
    }
    public async Task<(long ID,string Name)> ResolveCategoryAsync(long id,string name,string token,CancellationToken ct){if(id>0)return(id,name.Trim());name=name.Trim();if(name.Length==0)return(0,"");var list=await SearchCategoriesAsync(name,token,ct);if(list.Count==0)throw new InvalidOperationException($"Kick could not find a category named \"{name}\"");var exact=list.FirstOrDefault(x=>x.Name.Equals(name,StringComparison.OrdinalIgnoreCase));if(exact is not null)return(exact.ID,exact.Name);return(list[0].ID,list[0].Name);}
    public Task PatchChannelMetadataAsync(string token,string title,long categoryID,CancellationToken ct){var body=new Dictionary<string,object?>{{"stream_title",title}};if(categoryID>0)body["category_id"]=categoryID;return APIAsync(HttpMethod.Patch,APIBase+"/channels",token,JsonSerializer.SerializeToUtf8Bytes(body,AppUtil.Json),ct);}

    public async Task<bool> VerifyWebhookAsync(string messageID,string timestamp,byte[] body,string signatureB64,CancellationToken ct)
    {
        if(string.IsNullOrWhiteSpace(messageID)||string.IsNullOrWhiteSpace(timestamp)||string.IsNullOrWhiteSpace(signatureB64))return false; byte[] sig;try{sig=Convert.FromBase64String(signatureB64.Trim());}catch{return false;} var signed=Encoding.UTF8.GetBytes(messageID.Trim()+"."+timestamp.Trim()+"."+Encoding.UTF8.GetString(body));var hash=SHA256.HashData(signed);
        var key=await GetWebhookKeyAsync(false,ct); if(key.VerifyHash(hash,sig,HashAlgorithmName.SHA256,RSASignaturePadding.Pkcs1))return true;
        bool refresh;lock(keyGate)refresh=webhookKey!=null&&DateTime.UtcNow-webhookKeyAt>=TimeSpan.FromMinutes(5);if(refresh){key=await GetWebhookKeyAsync(true,ct);return key.VerifyHash(hash,sig,HashAlgorithmName.SHA256,RSASignaturePadding.Pkcs1);}return false;
    }

    public async Task<byte[]> FetchRawAsync(string url,string token,CancellationToken ct)
    {
        using var req=new HttpRequestMessage(HttpMethod.Get,url);if(token.Length>0)req.Headers.Authorization=new AuthenticationHeaderValue("Bearer",token);using var resp=await http.SendAsync(req,HttpCompletionOption.ResponseHeadersRead,ct);if(!resp.IsSuccessStatusCode)throw new InvalidOperationException($"remote request returned {(int)resp.StatusCode}");return await resp.Content.ReadAsByteArrayAsync(ct);
    }

    private async Task<RSA> GetWebhookKeyAsync(bool force,CancellationToken ct)
    {
        lock(keyGate){if(!force&&webhookKey!=null&&DateTime.UtcNow-webhookKeyAt<TimeSpan.FromHours(6))return webhookKey;}
        var data=await APIAsync(HttpMethod.Get,APIBase+"/public-key","",null,ct);using var doc=JsonDocument.Parse(data);var pem=doc.RootElement.GetProperty("data").GetProperty("public_key").GetString()?.Trim()??"";if(pem.Length==0)throw new InvalidOperationException("Kick returned an invalid webhook public key response");var rsa=RSA.Create();rsa.ImportFromPem(pem);lock(keyGate){webhookKey?.Dispose();webhookKey=rsa;webhookKeyAt=DateTime.UtcNow;return rsa;}
    }

    private async Task<byte[]> APIAsync(HttpMethod method,string url,string token,byte[]? body,CancellationToken ct)
    {
        using var req=new HttpRequestMessage(method,url);if(!string.IsNullOrWhiteSpace(token))req.Headers.Authorization=new AuthenticationHeaderValue("Bearer",token.Trim());if(body!=null)req.Content=new ByteArrayContent(body){Headers={ContentType=new MediaTypeHeaderValue("application/json")}};using var resp=await http.SendAsync(req,ct);var data=await resp.Content.ReadAsByteArrayAsync(ct);if(!resp.IsSuccessStatusCode){var msg=ExtractError(data,resp.ReasonPhrase??"Kick request failed");if(resp.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)throw new UnauthorizedAccessException("Kick rejected the request. Make sure your connected Kick credentials are allowed to edit this channel");if(resp.StatusCode==HttpStatusCode.NotFound)throw new InvalidOperationException("Kick could not find that channel or category");throw new InvalidOperationException(msg);}return data;
    }

    private static KickChannelMeta ParseChannelMetadata(byte[] data,string fallback)
    {
        using var doc=JsonDocument.Parse(data);if(!doc.RootElement.TryGetProperty("data",out var arr)||arr.ValueKind!=JsonValueKind.Array||arr.GetArrayLength()==0)throw new InvalidOperationException(fallback.Length>0?$"Kick did not return channel data for @{AppUtil.NormalizeKickChannelSlug(fallback)}":"Kick did not return channel data for the authorized account");var x=arr[0];var id=x.TryGetProperty("broadcaster_user_id",out var bi)&&bi.TryGetInt64(out var n)?n:0;var slug=x.TryGetProperty("slug",out var sl)?AppUtil.NormalizeKickChannelSlug(sl.GetString()):AppUtil.NormalizeKickChannelSlug(fallback);var title=x.TryGetProperty("stream_title",out var ti)?ti.GetString()?.Trim()??"":"";long cid=0;string cname="";if(x.TryGetProperty("category",out var c)&&c.ValueKind==JsonValueKind.Object){if(c.TryGetProperty("id",out var ci))ci.TryGetInt64(out cid);if(c.TryGetProperty("name",out var cn))cname=cn.GetString()?.Trim()??"";}var live=x.TryGetProperty("stream",out var st)&&st.ValueKind==JsonValueKind.Object&&st.TryGetProperty("is_live",out var li)&&li.ValueKind==JsonValueKind.True;return new(id,slug,title,cid,cname,live);
    }
    private static string ExtractError(byte[] data,string fallback){try{using var doc=JsonDocument.Parse(data);var r=doc.RootElement;if(r.TryGetProperty("message",out var m)&&!string.IsNullOrWhiteSpace(m.ToString()))return m.ToString().Trim();if(r.TryGetProperty("error",out var e)&&!string.IsNullOrWhiteSpace(e.ToString()))return e.ToString().Trim();}catch{}var raw=Encoding.UTF8.GetString(data).Trim();return raw.Length>0?raw:fallback;}
    public void Dispose(){http.Dispose();lock(keyGate){webhookKey?.Dispose();webhookKey=null;}}
}

internal sealed class KickUserAuthService
{
    private const string RedirectURI="http://127.0.0.1:17891/oauth/kick/callback";
    private const string Scopes="user:read channel:read channel:write";
    private readonly object gate=new();
    private readonly string path;
    private string accessToken="",refreshToken="",tokenType="",scope="",pendingState="",pendingVerifier="",lastError="";
    private DateTime expiresAt,pendingAt;

    public KickUserAuthService(string dataDir){path=Path.Combine(dataDir,"kick_user_authorization.json");Load();}
    public KickUserAuthState State(bool hasCreds){lock(gate){var pending=pendingState.Length>0&&DateTime.UtcNow-pendingAt<TimeSpan.FromMinutes(10);if(!pending&&pendingState.Length>0){pendingState=pendingVerifier="";pendingAt=default;}var auth=refreshToken.Length>0||(accessToken.Length>0&&(expiresAt==default||DateTime.UtcNow<expiresAt));return new(){Authorized=auth,Scope=scope,ExpiresAt=expiresAt==default?0:new DateTimeOffset(expiresAt).ToUnixTimeMilliseconds(),RedirectURI=RedirectURI,Pending=pending,LastError=lastError,HasAppCredentials=hasCreds};}}
    public string Begin(string clientID){clientID=clientID.Trim();if(clientID.Length==0)throw new InvalidOperationException("connect Kick on the Connections page first so SleepySource has your Developer App Client ID");var verifier=RandomURL(48);var state=RandomURL(32);lock(gate){pendingVerifier=verifier;pendingState=state;pendingAt=DateTime.UtcNow;lastError="";}var challenge=Base64Url(SHA256.HashData(Encoding.UTF8.GetBytes(verifier)));var q=new Dictionary<string,string>{{"response_type","code"},{"client_id",clientID},{"redirect","127.0.0.1"},{"redirect_uri",RedirectURI},{"scope",Scopes},{"code_challenge",challenge},{"code_challenge_method","S256"},{"state",state}};return KickService.OAuthBase+"/oauth/authorize?"+string.Join("&",q.Select(x=>Uri.EscapeDataString(x.Key)+"="+Uri.EscapeDataString(x.Value)));}
    public async Task FinishAsync(string code,string state,string clientID,string clientSecret,CancellationToken ct){string verifier,expected;DateTime at;lock(gate){verifier=pendingVerifier;expected=pendingState;at=pendingAt;}if(string.IsNullOrWhiteSpace(code))throw new InvalidOperationException("Kick did not return an authorization code");if(expected.Length==0||verifier.Length==0||state.Trim()!=expected||DateTime.UtcNow-at>TimeSpan.FromMinutes(10))throw new InvalidOperationException("Kick authorization session expired or did not match. Start authorization again");if(string.IsNullOrWhiteSpace(clientID)||string.IsNullOrWhiteSpace(clientSecret))throw new InvalidOperationException("saved Kick Developer App credentials are unavailable; reconnect Kick in Chat Overlay");var token=await RequestUserTokenAsync(new(){["grant_type"]="authorization_code",["code"]=code.Trim(),["client_id"]=clientID.Trim(),["client_secret"]=clientSecret.Trim(),["redirect_uri"]=RedirectURI,["code_verifier"]=verifier},ct);if(token.Refresh.Length==0)throw new InvalidOperationException("Kick OAuth did not return a refresh token");lock(gate){accessToken=token.Access;refreshToken=token.Refresh;tokenType=token.Type;scope=token.Scope;expiresAt=DateTime.UtcNow.AddSeconds(Math.Max(60,token.Expires));pendingState=pendingVerifier="";pendingAt=default;lastError="";}await SaveAsync();}
    public async Task<string> EnsureTokenAsync(string clientID,string clientSecret,CancellationToken ct){string access,refresh;DateTime expires;lock(gate){access=accessToken;refresh=refreshToken;expires=expiresAt;}if(access.Length>0&&(expires==default||DateTime.UtcNow.AddSeconds(45)<expires))return access;if(refresh.Length==0)throw new UnauthorizedAccessException("Authorize Stream Controls first");try{var token=await RequestUserTokenAsync(new(){["grant_type"]="refresh_token",["client_id"]=clientID.Trim(),["client_secret"]=clientSecret.Trim(),["refresh_token"]=refresh},ct);lock(gate){accessToken=token.Access;if(token.Refresh.Length>0)refreshToken=token.Refresh;tokenType=token.Type;if(token.Scope.Length>0)scope=token.Scope;expiresAt=DateTime.UtcNow.AddSeconds(Math.Max(60,token.Expires));lastError="";}await SaveAsync();return token.Access;}catch{lock(gate){lastError="Stream Controls authorization expired. Authorize again.";accessToken=refreshToken="";expiresAt=default;}try{File.Delete(path);}catch{}throw new UnauthorizedAccessException("Stream Controls authorization expired. Authorize again");}}
    public void Clear(){lock(gate){accessToken=refreshToken=tokenType=scope=pendingState=pendingVerifier=lastError="";expiresAt=pendingAt=default;}try{File.Delete(path);}catch{}}
    public string RefreshToken(){lock(gate)return refreshToken;}
    public void SetError(string message){lock(gate)lastError=(message??"").Trim();}

    private async Task<(string Access,string Refresh,string Type,string Scope,long Expires)> RequestUserTokenAsync(Dictionary<string,string> form,CancellationToken ct){using var http=new HttpClient{Timeout=TimeSpan.FromSeconds(15)};using var req=new HttpRequestMessage(HttpMethod.Post,KickService.OAuthBase+"/oauth/token"){Content=new FormUrlEncodedContent(form)};req.Headers.UserAgent.ParseAdd("SleepySource/1.0");using var resp=await http.SendAsync(req,ct);var data=await resp.Content.ReadAsByteArrayAsync(ct);if(!resp.IsSuccessStatusCode)throw new InvalidOperationException("Kick OAuth returned "+resp.StatusCode+": "+Encoding.UTF8.GetString(data).Trim());using var doc=JsonDocument.Parse(data);var r=doc.RootElement;var a=r.TryGetProperty("access_token",out var at)?at.GetString()?.Trim()??"":"";if(a.Length==0)throw new InvalidOperationException("Kick OAuth did not return a user access token");var refh=r.TryGetProperty("refresh_token",out var rt)?rt.GetString()?.Trim()??"":"";var typ=r.TryGetProperty("token_type",out var ty)?ty.GetString()?.Trim()??"":"";var sc=r.TryGetProperty("scope",out var s)?s.GetString()?.Trim()??"":"";long ex=3600;if(r.TryGetProperty("expires_in",out var ei)){if(ei.ValueKind==JsonValueKind.Number)ei.TryGetInt64(out ex);else long.TryParse(ei.GetString(),out ex);}return(a,refh,typ,sc,Math.Max(60,ex));}
    private void Load(){if(!OperatingSystem.IsWindows()||!File.Exists(path))return;try{using var doc=JsonDocument.Parse(File.ReadAllBytes(path));var enc=doc.RootElement.GetProperty("protected_data").GetString()??"";var plain=Encoding.UTF8.GetString(AppUtil.UnprotectCredential(Convert.FromBase64String(enc)));using var tok=JsonDocument.Parse(plain);var r=tok.RootElement;accessToken=r.TryGetProperty("access_token",out var a)?a.GetString()?.Trim()??"":"";refreshToken=r.TryGetProperty("refresh_token",out var f)?f.GetString()?.Trim()??"":"";tokenType=r.TryGetProperty("token_type",out var t)?t.GetString()?.Trim()??"":"";scope=r.TryGetProperty("scope",out var s)?s.GetString()?.Trim()??"":"";if(r.TryGetProperty("expires_at",out var e)&&e.TryGetInt64(out var sec)&&sec>0)expiresAt=DateTimeOffset.FromUnixTimeSeconds(sec).UtcDateTime;}catch{}}
    private Task SaveAsync(){if(!OperatingSystem.IsWindows())return Task.CompletedTask;string a,r,t,s;DateTime e;lock(gate){a=accessToken;r=refreshToken;t=tokenType;s=scope;e=expiresAt;}var raw=JsonSerializer.SerializeToUtf8Bytes(new{access_token=a,refresh_token=r,token_type=t,scope=s,expires_at=e==default?0:new DateTimeOffset(e).ToUnixTimeSeconds()},AppUtil.Json);var stored=new{version=1,protected_data=Convert.ToBase64String(AppUtil.ProtectCredential(raw))};return AppUtil.AtomicWriteJsonAsync(path,stored);}
    private static string RandomURL(int bytes){var b=RandomNumberGenerator.GetBytes(Math.Max(32,bytes));return Base64Url(b);}private static string Base64Url(byte[] b)=>Convert.ToBase64String(b).TrimEnd('=').Replace('+','-').Replace('/','_');
}
