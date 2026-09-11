const $=id=>document.getElementById(id);
async function render(){const {status={},profile,paused=false,gap}=await chrome.storage.local.get(['status','profile','paused','gap']);$('connection').textContent=status.detail??'Waiting for native bridge';$('profile').textContent=profile??'Initializing';$('paused').checked=paused;$('pair').textContent=status.paired?'Paired with local Heimdall':`Pair locally: heimdall browser pair ${profile??'PROFILE'}`;$('gap').textContent=gap??'';$('capture').style.display=status.paired?'':'none';}
$('paused').addEventListener('change',()=>chrome.storage.local.set({paused:$('paused').checked}));
$('capture').addEventListener('submit',async e=>{e.preventDefault();$('cap-status').textContent='';const [tab]=await chrome.tabs.query({active:true,currentWindow:true});if(!tab||!/^https?:/.test(tab.url??'')){$('cap-status').textContent='Active tab is not a capturable HTTP(S) page';return;}
 const target=$('cap-target').value.trim()||'unassigned',kind=$('cap-kind').value,why=$('cap-why').value.trim();
 const line=`${target}/${kind} ${why}`.trimEnd();
 try{const r=await chrome.runtime.sendMessage({type:'heimdall-capture',capture:{line,pointer:tab.url,title:tab.title??''}});$('cap-status').textContent=r&&r.capture_id?`Captured ${r.capture_id}`:`Error: ${r&&r.error||'no reply'}`;if(r&&r.capture_id)$('cap-why').value='';}catch(err){$('cap-status').textContent='Error: '+String(err.message||err);}});
chrome.storage.onChanged.addListener(render);render();
