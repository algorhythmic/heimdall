// Sampled attention only. No span survives lost collection coverage, a paused
// observer or a worker restart. Short visits are dropped rather than attributed
// to an adjacent tab. Timestamps are source observation times, not input times.
export class FocusTracker {
  constructor(){this.reset();}
  reset(){this.current=null;this.last=null;}
  observe(snapshot){
    const at=Date.parse(snapshot.observed_at);
    if(!snapshot.complete||!Number.isFinite(at)||(this.last!==null&&at<this.last)){this.reset();return [];}
    const matches=snapshot.tabs.filter(t=>t.active&&t.window_id===snapshot.focused_window);
    if(matches.length>1||matches.some(t=>t.navigation_pending)){this.reset();return [];}
    // A focused window with no allowed active tab is a content coverage gap.
    if(snapshot.focused_window>0&&!matches.length){this.reset();return [];}
    const tab=matches[0];
    if(tab){try{decodeURIComponent(new URL(tab.url).search);}catch{this.reset();return [];}}
    const next=tab?{tab_id:tab.id,window_id:tab.window_id,pointer:tab.url,started_at:snapshot.observed_at}:null;
    const old=this.current;
    this.last=at;
    if(old&&next&&old.tab_id===next.tab_id&&old.window_id===next.window_id&&old.pointer===next.pointer)return [];
    this.current=next;
    if(!old)return [];
    const seconds=(at-Date.parse(old.started_at))/1000;
    if(seconds<2||seconds>86400)return [];
    return [{version:1,...old,ended_at:snapshot.observed_at,duration_s:seconds}];
  }
}
