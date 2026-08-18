namespace SleepySource;

internal sealed class HealthService
{
    private readonly CoreStateService core;
    private readonly ChatService chat;
    private readonly AlertService alerts;
    private readonly SleepySourceApiService sleepyApi;
    private readonly HttpClient http = new() { Timeout = TimeSpan.FromSeconds(4) };

    public HealthService(CoreStateService core, ChatService chat, AlertService alerts, SleepySourceApiService sleepyApi)
    {
        this.core = core; this.chat = chat; this.alerts = alerts; this.sleepyApi = sleepyApi;
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
        if (ContainsError(state.Diagnostics.Status)) checks.Add(C("now-playing","Core System","Now Playing Detector","warn",state.Diagnostics.Status));
        else if (state.Track.Found) checks.Add(C("now-playing","Core System","Now Playing Detector","pass","Media detection is active" + (string.IsNullOrWhiteSpace(state.DisplayText) ? "." : ": " + state.DisplayText + ".")));
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

        var apiOnline = await sleepyApi.CheckApiHealthAsync(ct);
        checks.Add(apiOnline
            ? C("sleepysource-api","Kick Connection","SleepySource API","pass","The hosted SleepySource authentication and event service is online.")
            : C("sleepysource-api","Kick Connection","SleepySource API","fail","The hosted SleepySource API could not be reached. Kick-powered features may be temporarily unavailable."));

        var hosted = sleepyApi.State();
        if (hosted.Connected && hosted.KickUsername.Length > 0)
            checks.Add(C("kick-auth","Kick Connection","Kick Authentication","pass","Connected securely as @"+hosted.KickUsername+"."));
        else if (hosted.OAuthPending)
            checks.Add(C("kick-auth","Kick Connection","Kick Authentication","info","Waiting for Kick authorization to finish in your browser."));
        else
            checks.Add(R("kick-auth","Kick Connection","Kick Authentication","warn","Kick is not connected. Connect once to enable Chat Overlay, Alert Studio, and Stream Dashboard Kick features.","open-connections","Open Connections"));

        if (!hosted.Connected)
            checks.Add(C("kick-events","Kick Connection","Kick Event Subscriptions","info","Event subscriptions activate automatically after Kick is connected."));
        else if (hosted.EventsReady)
            checks.Add(C("kick-events","Kick Connection","Kick Event Subscriptions","pass","Required Kick chat and alert events are managed automatically by the hosted service."));
        else
            checks.Add(R("kick-events","Kick Connection","Kick Event Subscriptions","warn","Kick event subscriptions need attention"+(hosted.LastError.Length>0?": "+hosted.LastError:"."),"sync-kick-events","Sync Kick Events"));

        if (!hosted.Connected)
            checks.Add(C("kick-realtime","Kick Connection","Realtime Delivery","info","Realtime event delivery starts automatically after Kick is connected."));
        else if (hosted.RealtimeStatus == "connected")
            checks.Add(C("kick-realtime","Kick Connection","Realtime Delivery","pass","Realtime Kick event delivery is connected."));
        else
            checks.Add(C("kick-realtime","Kick Connection","Realtime Delivery","warn","Realtime is "+hosted.RealtimeStatus+". The reliable hosted fallback queue remains available."));

        checks.Add(hosted.FallbackQueue
            ? C("kick-fallback","Kick Connection","Event Delivery Queue","pass","Reliable hosted fallback delivery is enabled, so temporary realtime interruptions do not lose queued events.")
            : C("kick-fallback","Kick Connection","Event Delivery Queue","warn","The reliable fallback queue is not reported ready."));

        checks.Add(hosted.DeliveredEvents > 0
            ? C("kick-activity","Kick Connection","Kick Event Activity","pass",$"{hosted.DeliveredEvents} hosted Kick events delivered. Last event: {hosted.LastEventType}.")
            : C("kick-activity","Kick Connection","Kick Event Activity","info","No hosted Kick events have been delivered to this app session yet."));

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
