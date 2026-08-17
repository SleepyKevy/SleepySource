using System.Text.Json;

namespace SleepySource;

internal static class KickAlertParser
{
    private static readonly Dictionary<string,string> Types=new(StringComparer.OrdinalIgnoreCase)
    {
        ["channel.followed"]="follow",["channel.subscription.new"]="subscription-new",["channel.subscription.renewal"]="subscription-renewal",["channel.subscription.gifts"]="subscription-gift",["kicks.gifted"]="kicks",["channel.reward.redemption.updated"]="reward"
    };
    public static bool IsSupported(string type)=>Types.ContainsKey((type??"").Trim());

    public static (AlertEvent Event,string Dedupe) Parse(string eventType,string messageId,byte[] body,string connectedBroadcaster)
    {
        if(!Types.TryGetValue((eventType??"").Trim(),out var type))throw new InvalidDataException("unsupported Kick alert event");
        using var doc=JsonDocument.Parse(body);var r=doc.RootElement;
        if(!string.IsNullOrWhiteSpace(connectedBroadcaster)&&r.TryGetProperty("broadcaster",out var b)&&TryLong(b,"user_id",out var bid)&&bid>0&&!connectedBroadcaster.Equals(bid.ToString(),StringComparison.Ordinal))throw new InvalidDataException("Kick alert broadcaster does not match the connected channel");
        var ev=new AlertEvent{ID=messageId.Trim(),Type=type,Source="kick",CreatedAtMS=AppUtil.NowMS()};var dedupe="kick:"+messageId.Trim();
        switch(type)
        {
            case "follow": ev.Username=UserName(Get(r,"follower")); break;
            case "subscription-new":
            case "subscription-renewal": ev.Username=UserName(Get(r,"subscriber"));ev.Months=Int(r,"duration");ev.CreatedAtMS=Time(r,"created_at",ev.CreatedAtMS);break;
            case "subscription-gift": var g=Get(r,"gifter");ev.Username=UserName(g);if(g.ValueKind==JsonValueKind.Object&&Bool(g,"is_anonymous"))ev.Username="Anonymous";if(r.TryGetProperty("giftees",out var gs)&&gs.ValueKind==JsonValueKind.Array)ev.Count=gs.GetArrayLength();ev.CreatedAtMS=Time(r,"created_at",ev.CreatedAtMS);break;
            case "kicks": ev.Username=UserName(Get(r,"sender"));var gift=Get(r,"gift");ev.Amount=Int(gift,"amount");ev.GiftName=String(gift,"name");if(ev.GiftName.Length==0)ev.GiftName="Kicks Gift";ev.CreatedAtMS=Time(r,"created_at",ev.CreatedAtMS);break;
            case "reward": var id=String(r,"id");if(id.Length==0)throw new InvalidDataException("invalid reward alert payload");ev.Username=UserName(Get(r,"redeemer"));var rw=Get(r,"reward");ev.RewardTitle=String(rw,"title");if(ev.RewardTitle.Length==0)ev.RewardTitle="Channel Reward";ev.UserInput=String(r,"user_input");ev.CreatedAtMS=Time(r,"redeemed_at",ev.CreatedAtMS);dedupe="kick:reward:"+id;break;
        }
        if(string.IsNullOrWhiteSpace(ev.Username))ev.Username="Anonymous";if(ev.Count<0||ev.Amount<0||ev.Months<0)throw new InvalidDataException("invalid alert payload");return(ev,dedupe);
    }
    private static JsonElement Get(JsonElement r,string name)=>r.ValueKind==JsonValueKind.Object&&r.TryGetProperty(name,out var x)?x:default;
    private static string String(JsonElement r,string name)=>Get(r,name).ValueKind==JsonValueKind.String?Get(r,name).GetString()?.Trim()??"":"";
    private static int Int(JsonElement r,string name){var x=Get(r,name);if(x.ValueKind==JsonValueKind.Number&&x.TryGetInt32(out var n))return n;if(x.ValueKind==JsonValueKind.String&&int.TryParse(x.GetString(),out n))return n;return 0;}
    private static bool Bool(JsonElement r,string name){var x=Get(r,name);return x.ValueKind==JsonValueKind.True||(x.ValueKind==JsonValueKind.String&&bool.TryParse(x.GetString(),out var b)&&b);}
    private static bool TryLong(JsonElement r,string name,out long n){var x=Get(r,name);if(x.ValueKind==JsonValueKind.Number)return x.TryGetInt64(out n);if(x.ValueKind==JsonValueKind.String)return long.TryParse(x.GetString(),out n);n=0;return false;}
    private static string UserName(JsonElement user){if(user.ValueKind!=JsonValueKind.Object)return"Anonymous";if(Bool(user,"is_anonymous"))return"Anonymous";var n=String(user,"username");return n.Length==0?"Anonymous":n;}
    private static long Time(JsonElement r,string name,long fallback){var s=String(r,name);return DateTimeOffset.TryParse(s,out var t)?t.ToUnixTimeMilliseconds():fallback;}
}
