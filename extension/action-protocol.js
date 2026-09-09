export const executionStates = ['queued','dispatching','api_reported','refused','uncertain','cancelled'];
export const verificationStates = ['pending','matched','not_matched','unknown','unsupported'];
export function validActionRef(r){
  if(!r||r.version!==1||!/^[a-z0-9][a-z0-9-]{1,22}[a-z0-9]$/.test(r.target)||['unassigned','radiator'].includes(r.target))return false;
  if(Object.keys(r).sort().join(',')!==['version','id','attempt_id','intent_digest','target','manifest_id','surface_id'].sort().join(','))return false;
  return ['id','attempt_id','manifest_id','surface_id'].every(k=>typeof r[k]==='string'&&/^[a-f0-9]{32}$/.test(r[k]))&&typeof r.intent_digest==='string'&&/^[a-f0-9]{64}$/.test(r.intent_digest);
}
export function actionResult(op,result){return op.action_ref?{...result,action_ref:op.action_ref}:result;}
export function requestFingerprint(op){
  const r=op.action_ref,ref=r?[r.version,r.id,r.attempt_id,r.intent_digest,r.target,r.manifest_id,r.surface_id]:null;
  const base=[op.id,op.epoch,op.action,op.tab_id??0,op.window_id??0,op.owner_id??'',op.expected_url??'',op.url??'',op.expires_at,ref];
  if(op.pairing)base.push([op.pairing.version,op.pairing.source_id,op.pairing.source_epoch,op.pairing.previous_viewport]);
  return JSON.stringify(base);
}
