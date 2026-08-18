$ErrorActionPreference = 'Stop'

$formPath = '_sleepymusic_build/CSharpHost/MainForm.cs'
$uiPath = '_sleepymusic_build/CSharpHost/web/index.html'

$form = Get-Content $formPath -Raw
$oldForm = '        var json=JsonSerializer.Serialize(new { requestId, ok, cancelled, payload },AppUtil.Json);'
$newForm = @'
        var envelope = new Dictionary<string, object?>
        {
            ["requestId"] = requestId,
            ["ok"] = ok,
            ["cancelled"] = cancelled,
            ["payload"] = payload
        };
        var json=JsonSerializer.Serialize(envelope,AppUtil.Json);
'@.TrimEnd()
if (-not $form.Contains($oldForm)) { throw 'Expected MainForm native envelope line was not found.' }
$form = $form.Replace($oldForm, $newForm)
Set-Content -Path $formPath -Value $form -Encoding utf8

$ui = Get-Content $uiPath -Raw
$oldUi = "window.sleepyNativeResult=result=>{const p=nativePending.get(result?.requestId);if(!p)return;nativePending.delete(result.requestId);if(result.ok)p.resolve(result);else p.reject(new Error(String(result.payload||'Native request failed')))};"
$newUi = "window.sleepyNativeResult=result=>{const requestId=result?.requestId||result?.request_id;const p=nativePending.get(requestId);if(!p)return;nativePending.delete(requestId);if(result.ok)p.resolve(result);else p.reject(new Error(String(result.payload||'Native request failed')))};"
if (-not $ui.Contains($oldUi)) { throw 'Expected UI native result handler was not found.' }
$ui = $ui.Replace($oldUi, $newUi)
Set-Content -Path $uiPath -Value $ui -Encoding utf8

$formCheck = Get-Content $formPath -Raw
$uiCheck = Get-Content $uiPath -Raw
if (-not $formCheck.Contains('["requestId"] = requestId')) { throw 'Camel-case requestId envelope fix is missing.' }
if (-not $uiCheck.Contains('result?.request_id')) { throw 'UI snake_case compatibility fix is missing.' }
Write-Host 'SleepyMusic native response ID contract patched.'
