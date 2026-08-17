namespace SleepySource;

internal sealed class HealthService
{
    private readonly CoreStateService core;
    private readonly ChatService chat;
    private readonly AlertService alerts;
    private readonly CloudflareService cloudflare;
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(4) };

    public HealthService(CoreStateService core, ChatService chat, AlertService alerts, CloudflareService cloudflare)
    {
        this.core = core; this.chat = chat; this.alerts = alerts; this.cloudflare = cloudflare;
    }

    public async Task<HealthReport> RunAsync(CancellationToken ct)
    {
        var checks = new List<HealthCheck>
        {
            C("local-server","Core System","Local Server","pass","SleepySource is responding on 127.0.0.1:17891.")
        };
        AddWritable(checks,"data-storage","Core System","Settings & Data Storage",core.DataDir,"Settings and portable data storage are writable.");
        AddWritable(checks,"media-storage","Core System","Media Storage",core.MediaDir,"Custom artwork, backgrounds, and uploaded media storage are writable.");
        AddWritable(checks,"alert-media-storage","Alert Studio","Alert Media Storage",alerts.MediaDir,"Alert Studio image, video, and sound storage is writable.");
        AddWritable(checks,"profiles","Profiles","Profile Storage",core.ProfileDir,"Profile storage is writable.");

        var state = core.Snapshot();
        if (ContainsError(state.Diagnostics.Status))
            checks.Add(C("now-playing","Core System","Now Playing Detector","warn",state.Diagnostics.Status));
        else if (state.Track.Found)
            checks.Add(C("now-playing","Core System","Now Playing Detector","pass","Media detection is active" + (string.IsNullOrWhiteSpace(state.DisplayText) ? "." : ": " + state.DisplayText + ".")));
        else checks.Add(C("now-playing","Core System","Now Playing Detector","pass","Media detector is running; nothing is currently playing."));

        foreach (var (id,name,path) in new[]{("overlay-now-playing","Now Playing Overlay","/overlay"),("overlay-chat","Chat Overlay","/chat"),("overlay-alerts","Alert Studio Overlay","/alerts"),("overlay-countdown","Countdown Pro Overlay","/countdown")})
        {
            try
            {
                using var req = new HttpRequestMessage(HttpMethod.Head, "http://127.0.0.1:17891" + path);
                using var resp = await http.SendAsync(req, ct);
                checks.Add(resp.IsSuccessStatusCode || (int)resp.StatusCode < 400 ? C(id,"OBS Overlays",name,"pass",path+" is responding normally.") : C(id,"OBS Overlays",name,"fail",path+" returned HTTP "+(int)resp.StatusCode));
            }
            catch (Exception ex) { checks.Add(C(id,"OBS Overlays",name,"fail",path+" did not respond: "+ex.Message)); }
        }

        var cs = chat.State();
        if (cs.AuthReady && !string.IsNullOrWhiteSpace(cs.ConnectedChannel)) checks.Add(C("kick-auth","Kick Connection","Kick Authentication","pass","Connected to @"+cs.ConnectedChannel+"."));
        else if (cs.AuthReady) checks.Add(R("kick-auth","Kick Connection","Kick Authentication","warn","Kick credentials are available, but no channel is connected.","open-connections","Open Connections"));
        else checks.Add(R("kick-auth","Kick Connection","Kick Authentication","warn","Kick is not connected. This only affects Kick-powered features.","open-connections","Connect Kick"));

        if (string.IsNullOrWhiteSpace(cs.ConnectedChannel)) checks.Add(C("kick-webhook","Kick Connection","Kick Webhook Subscription","info","Connect a Kick channel to enable official chat and Alert Studio events."));
        else if (cs.WebhookSubscribed) checks.Add(C("kick-webhook","Kick Connection","Kick Webhook Subscription","pass","Official Kick chat and Alert Studio events are registered."));
        else checks.Add(R("kick-webhook","Kick Connection","Kick Webhook Subscription","fail","The connected Kick channel does not currently have confirmed chat and Alert Studio webhook subscriptions."+(string.IsNullOrWhiteSpace(cs.WebhookLastError)?"":" Last error: "+cs.WebhookLastError),"reregister-kick","Repair Subscription"));

        if (cs.WebhookRequestCount == 0) checks.Add(C("kick-activity","Kick Connection","Webhook Activity","info","No webhook requests have been received yet. Send a Kick chat message or trigger a supported alert event after connecting to verify delivery."));
        else checks.Add(C("kick-activity","Kick Connection","Webhook Activity",cs.WebhookRejectedCount>0&&cs.WebhookAcceptedCount==0?"warn":"pass",$"{cs.WebhookRequestCount} requests • {cs.WebhookVerifiedCount} verified • {cs.WebhookAcceptedCount} accepted • {cs.WebhookRejectedCount} rejected."));

        var relay = cloudflare.HealthSnapshot();
        bool running = relay.Running; string publicUrl = relay.PublicURL; string lastError = relay.LastError; bool runtimeReady = relay.RuntimeReady; string runtimeVersion = relay.RuntimeVersion;
        checks.Add(runtimeReady ? C("relay-runtime","Relay","Cloudflare Runtime","pass","Managed Cloudflare runtime "+runtimeVersion+" is ready.") : C("relay-runtime","Relay","Cloudflare Runtime",running?"warn":"info",running?"Relay is starting, but the managed runtime is not yet reported ready.":"Managed Cloudflare runtime will be downloaded and verified automatically when the relay starts."));
        if (running && !string.IsNullOrWhiteSpace(publicUrl))
        {
            checks.Add(C("relay-process","Relay","Relay Process","pass","Cloudflare Quick Tunnel is running and has a public URL."));
            try { using var resp=await http.GetAsync(publicUrl.TrimEnd('/')+"/api/relay-health",ct); checks.Add(resp.IsSuccessStatusCode?C("relay-end-to-end","Relay","End-to-End Tunnel","pass","The public Cloudflare URL reaches this SleepySource instance."):R("relay-end-to-end","Relay","End-to-End Tunnel","warn","The relay is running, but the public probe returned HTTP "+(int)resp.StatusCode,"restart-relay","Restart Relay")); }
            catch(Exception ex){checks.Add(R("relay-end-to-end","Relay","End-to-End Tunnel","warn","The relay is running, but the public probe did not reach SleepySource: "+ex.Message,"restart-relay","Restart Relay"));}
        }
        else if (running) checks.Add(R("relay-process","Relay","Relay Process","warn",string.IsNullOrWhiteSpace(lastError)?"Relay process is running but is still waiting for a public URL.":"Relay needs attention: "+lastError,"restart-relay","Restart Relay"));
        else checks.Add(R("relay-process","Relay","Relay Process",string.IsNullOrWhiteSpace(lastError)?"info":"warn",string.IsNullOrWhiteSpace(lastError)?"Relay is stopped. Start it when Kick webhook delivery is needed.":"Relay is stopped. Last error: "+lastError,"start-relay","Start Relay"));

        var overall = checks.Any(x=>x.Status=="fail") ? "problem" : checks.Any(x=>x.Status=="warn") ? "attention" : "healthy";
        return new HealthReport { Version=AppUtil.DisplayVersion, CheckedAt=AppUtil.NowMS(), OverallStatus=overall, Summary=overall=="problem"?"One or more checks failed and may affect streaming features.":overall=="attention"?"SleepySource is running, but one or more items need attention.":"All checked systems are ready.", Checks=checks };
    }

    private static void AddWritable(List<HealthCheck> checks,string id,string group,string name,string dir,string good)
    {
        try { Directory.CreateDirectory(dir); var p=Path.Combine(dir,".sleepysource-health-"+Guid.NewGuid().ToString("N")); File.WriteAllText(p,"ok"); File.Delete(p); checks.Add(C(id,group,name,"pass",good)); }
        catch(Exception ex){checks.Add(C(id,group,name,"fail","SleepySource cannot write to its data folder: "+ex.Message));}
    }
    private static bool ContainsError(string s)=>new[]{"could not","missing","unavailable","failed","error"}.Any(x=>(s??"").Contains(x,StringComparison.OrdinalIgnoreCase));
    private static HealthCheck C(string id,string g,string n,string s,string m)=>new(){ID=id,Group=g,Name=n,Status=s,Message=m};
    private static HealthCheck R(string id,string g,string n,string s,string m,string a,string l)=>new(){ID=id,Group=g,Name=n,Status=s,Message=m,RepairAction=a,RepairLabel=l};
}
