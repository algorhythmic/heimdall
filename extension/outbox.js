export class Outbox {
  async db() {
    return this.opened ??= new Promise((resolve, reject) => {
      const r = indexedDB.open('heimdall-outbox', 1);
      r.onupgradeneeded = () => r.result.createObjectStore('records', {keyPath:'id'});
      r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error);
    });
  }
  async transaction(mode, fn) {
    const db = await this.db();
    return new Promise((resolve,reject) => {
      const tx=db.transaction('records',mode); const result=fn(tx.objectStore('records'));
      tx.oncomplete=()=>resolve(result?.result); tx.onerror=()=>reject(tx.error); tx.onabort=()=>reject(tx.error);
    });
  }
  all(){return this.transaction('readonly',s=>s.getAll());}
  remove(id){return this.transaction('readwrite',s=>s.delete(id));}
  clear(){return this.transaction('readwrite',s=>s.clear());}
  async put(record){
    const rows=await this.all(); const size=r=>new TextEncoder().encode(JSON.stringify(r)).length;let bytes=size(record);
    const expired=[];
    for(const row of rows.sort((a,b)=>b.created-a.created)) {
      bytes+=size(row);
      if(Date.now()-row.created>86400000 || bytes>32*1024*1024) expired.push(row.id);
    }
    await this.transaction('readwrite',s=>{for(const id of expired)s.delete(id);s.put(record);});
    return expired.length;
  }
}
