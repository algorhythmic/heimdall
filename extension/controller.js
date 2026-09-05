export const id = () => crypto.randomUUID().replaceAll('-','');
export function validURL(value){try{const u=new URL(value);return ['http:','https:'].includes(u.protocol)&&!u.username&&!u.password&&value.length<=8192;}catch{return false;}}
function utf8Trim(s,max){while(new TextEncoder().encode(s).length>max)s=s.slice(0,Math.floor(s.length*0.9));return s;}
export function inventory(tabs,focusedWindow){
  const result={type:'inventory',observed_at:new Date().toISOString(),tabs:[],focused_window:focusedWindow,complete:true};
  for(const t of tabs){if(t.incognito||!validURL(t.url)||t.id<1||t.windowId<1)continue;
    const row={id:t.id,window_id:t.windowId,url:t.url,title:utf8Trim(t.title??'',1024),active:!!t.active};
    result.tabs.push(row);
    if(result.tabs.length>2048||new TextEncoder().encode(JSON.stringify(result)).length>240*1024){result.tabs.pop();result.complete=false;break;}
  }return result;
}
export class Actions {
  constructor(api,epoch){this.api=api;this.epoch=epoch;}
  async execute(op,paused=false){
    const {journal={},owners={}}=await this.api.storage.session.get(['journal','owners']);
    const old=journal[op.id];
    if(old)return old.result??{operation_id:op.id,status:'uncertain',detail:'Previous attempt interrupted; action was not repeated'};
    const refuse=detail=>({operation_id:op.id,status:'refused',detail});
    if(paused||op.epoch!==this.epoch||Date.now()>=Date.parse(op.expires_at))return refuse('Paused, stale epoch, or expired');
    if(['open','navigate'].includes(op.action)&&!validURL(op.url))return refuse('URL is not allowed');
    if(!['open','navigate','focus','move','close'].includes(op.action))return refuse('Unknown action');
    let tab;
    if(op.action!=='open'){
      try{tab=await this.api.tabs.get(op.tab_id);}catch{return refuse('Tab no longer exists');}
      if(tab.incognito||!owners[tab.id]||owners[tab.id]!==op.owner_id||tab.url!==op.expected_url)return refuse('Tab ownership or URL changed');
      if(op.action==='move'){try{const w=await this.api.windows.get(op.window_id);if(w.incognito)return refuse('Private window');}catch{return refuse('Window no longer exists');}}
    }
    // Persist before any browser side effect. A interrupted attempt is never repeated.
    journal[op.id]={started:Date.now()};
    for(const key of Object.keys(journal)){if(journal[key].started<Date.now()-86400000)delete journal[key];}
    await this.api.storage.session.set({journal});
    let result;
    try{
      if(op.action==='open'){
        const w=await this.api.windows.create({url:this.api.runtime.getURL('launch.html')+'#'+op.id,focused:true,incognito:false});
        tab=w.tabs?.[0];if(!tab?.id)throw Error('Created window did not return tab');
        owners[tab.id]=op.id;await this.api.storage.session.set({owners});
        await this.api.tabs.update(tab.id,{url:op.url});
        result={operation_id:op.id,status:'succeeded',tab_id:tab.id,window_id:w.id,url:op.url};
      }else{
        if(op.action==='navigate')await this.api.tabs.update(tab.id,{url:op.url});
        if(op.action==='focus'){await this.api.tabs.update(tab.id,{active:true});await this.api.windows.update(tab.windowId,{focused:true});}
        if(op.action==='move')await this.api.tabs.move(tab.id,{windowId:op.window_id,index:-1});
        if(op.action==='close'){await this.api.tabs.remove(tab.id);delete owners[tab.id];await this.api.storage.session.set({owners});}
        result={operation_id:op.id,status:'succeeded'};
      }
    }catch(e){result={operation_id:op.id,status:'uncertain',detail:utf8Trim(String(e.message),512)};}
    journal[op.id].result=result;await this.api.storage.session.set({journal});return result;
  }
}
