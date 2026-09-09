const nonce=location.hash.slice(1);
if(/^[a-f0-9]{32}$/.test(nonce)){
 document.title='Heimdall pairing '+nonce;
 document.getElementById('nonce').textContent='Pairing reference: '+nonce;
 document.getElementById('cancel').addEventListener('click',async()=>{const tab=await chrome.tabs.getCurrent();if(tab?.id)await chrome.tabs.remove(tab.id);});
}else document.getElementById('cancel').disabled=true;
