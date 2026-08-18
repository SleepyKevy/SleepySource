$ErrorActionPreference = 'Stop'
$app = '_sleepymusic_build/current/SleepyMusic'
$uiPath = Join-Path $app 'web/index.html'
$backendPath = Join-Path $app 'BackendHost.cs'

$ui = Get-Content $uiPath -Raw

# Remove the old browser media element regardless of attribute order/whitespace.
$ui = [regex]::Replace($ui, '<audio\b[^>]*>\s*</audio>', '', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)

$oldAudioInit = "const `$=s=>document.querySelector(s),`$`$=s=>[...document.querySelectorAll(s)];const audio=`$('#audio');let lib={tracks:[],favorites:[],recent_track_ids:[],playlists:[],folders:[],files:[],settings:{theme:'blue',volume:.8,shuffle:false,repeat:'off'}};let view='home',queue=[],queuePos=-1,current=null,filter=null,saveTimer=null;"
$newAudioInit = "const `$=s=>document.querySelector(s),`$`$=s=>[...document.querySelectorAll(s)];const nativePlayer={paused:true,currentTime:0,duration:0,volume:.8,trackId:null};const audio={get paused(){return nativePlayer.paused},get currentTime(){return nativePlayer.currentTime},set currentTime(v){nativePlayer.currentTime=Number(v)||0;playerPost('player-seek',{seconds:nativePlayer.currentTime})},get duration(){return nativePlayer.duration},get volume(){return nativePlayer.volume},set volume(v){nativePlayer.volume=Math.max(0,Math.min(1,Number(v)||0));playerPost('player-volume',{value:nativePlayer.volume})},play(){nativePlayer.paused=false;playerPost('player-play');audio.onplay?.();return Promise.resolve()},pause(){nativePlayer.paused=true;playerPost('player-pause');audio.onpause?.()},ontimeupdate:null,onloadedmetadata:null,onplay:null,onpause:null,onended:null,onerror:null};function playerPost(type,extra={}){if(window.chrome?.webview?.postMessage)window.chrome.webview.postMessage({type,...extra})}let lib={tracks:[],favorites:[],recent_track_ids:[],playlists:[],folders:[],files:[],settings:{theme:'blue',volume:.8,shuffle:false,repeat:'off'}};let view='home',queue=[],queuePos=-1,current=null,filter=null,saveTimer=null;"
if ($ui.Contains($oldAudioInit)) { $ui = $ui.Replace($oldAudioInit, $newAudioInit) }
elseif (-not $ui.Contains('const nativePlayer=')) { throw 'Could not replace browser audio initialization.' }

$oldSelect = "function selectTrack(t,autoplay=true,markRecent=true){current=t;audio.src='/api/audio/'+encodeURIComponent(t.id);if(markRecent){fetch('/api/recent/'+encodeURIComponent(t.id),{method:'POST'});lib.recent_track_ids=[t.id,...lib.recent_track_ids.filter(x=>x!==t.id)].slice(0,40)}lib.settings.last_track_id=t.id;lib.settings.last_position_seconds=0;scheduleSettings();render();if(autoplay)audio.play().catch(()=>toast('This audio format could not be played'))}"
$newSelect = "function selectTrack(t,autoplay=true,markRecent=true){current=t;if(markRecent){fetch('/api/recent/'+encodeURIComponent(t.id),{method:'POST'});lib.recent_track_ids=[t.id,...lib.recent_track_ids.filter(x=>x!==t.id)].slice(0,40);lib.settings.last_position_seconds=0}const startPos=markRecent?0:Number(lib.settings.last_position_seconds||0);lib.settings.last_track_id=t.id;scheduleSettings();render();playerPost('player-load',{trackId:t.id,autoplay,position:startPos})}"
if ($ui.Contains($oldSelect)) { $ui = $ui.Replace($oldSelect, $newSelect) }
elseif (-not $ui.Contains("playerPost('player-load'")) { throw 'Could not replace browser track loading.' }

$oldMedia = "window.sleepyMediaKey=cmd=>{if(cmd==='playpause')`$('#play').click();else if(cmd==='next')nextTrack(false);else if(cmd==='previous')nextTrack(true);else if(cmd==='stop'){audio.pause();audio.currentTime=0}};"
$nativeCallbacks = "window.sleepyNativePlayerState=s=>{const wasPaused=nativePlayer.paused;const oldDuration=nativePlayer.duration;nativePlayer.trackId=s?.trackId||null;nativePlayer.paused=!s?.playing;nativePlayer.currentTime=Number(s?.position||0);nativePlayer.duration=Number(s?.duration||0);if(Number.isFinite(Number(s?.volume)))nativePlayer.volume=Number(s.volume);if(oldDuration!==nativePlayer.duration&&nativePlayer.duration>0)audio.onloadedmetadata?.();audio.ontimeupdate?.();if(wasPaused!==nativePlayer.paused)(nativePlayer.paused?audio.onpause:audio.onplay)?.()};window.sleepyNativePlayerEnded=()=>audio.onended?.();window.sleepyNativePlayerError=msg=>{console.error(msg);toast(String(msg||'Native playback failed'));audio.onerror?.()};" + $oldMedia
if ($ui.Contains($oldMedia) -and -not $ui.Contains('window.sleepyNativePlayerState=')) { $ui = $ui.Replace($oldMedia, $nativeCallbacks) }
elseif (-not $ui.Contains('window.sleepyNativePlayerState=')) { throw 'Could not install native player callbacks.' }

if ($ui.Contains('<audio')) { throw 'HTML audio element still exists after native patch.' }
if ($ui.Contains('/api/audio/')) { throw 'Browser audio endpoint still exists in UI after native patch.' }
if (-not $ui.Contains('result?.requestId||result?.request_id')) { throw 'Individual-file picker response compatibility was lost.' }
Set-Content $uiPath $ui -Encoding utf8

$backend = Get-Content $backendPath -Raw
if (-not $backend.Contains('public TrackInfo? GetTrack(string id)')) {
    $anchor = '    public async Task<LibrarySnapshot> AddFolderFromHostAsync(string folder)'
    if (-not $backend.Contains($anchor)) { throw 'Could not find BackendHost host-library API anchor.' }
    $backend = $backend.Replace($anchor, "    public TrackInfo? GetTrack(string id) => library.Track(id);`r`n`r`n" + $anchor)
    Set-Content $backendPath $backend -Encoding utf8
}

Write-Host 'SleepyMusic browser playback replaced with native-player UI bridge.'
