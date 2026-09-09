import type {BrowserActionRef,BrowserOperationResult} from './action-protocol.js';
export interface BrowserPairingIntent {version:1;source_id:string;source_epoch:string;previous_viewport:string}
export interface BrowserPairReady {action_ref:BrowserActionRef;marker_tab_id:number;window_id:number;original_tab_id?:number}
export interface BrowserMarker {action_ref:BrowserActionRef;tab_id:number;window_id:number;active:boolean}
export interface BrowserPairOperation {id:string;action_ref:BrowserActionRef;profile:string;epoch:string;action:'open'|'associate';pairing:BrowserPairingIntent;expires_at:string;url?:string;tab_id?:number;window_id?:number;owner_id?:string;expected_url?:string}
export interface BrowserContinuation {version:1;id:string;action_ref:BrowserActionRef;profile:string;epoch:string;marker_tab_id:number;window_id:number;original_tab_id?:number;action:'open'|'associate';url?:string;expires_at:string}
export function pairURL(api:unknown,id:string):string;
export function retainedPairings(api:unknown,refs:BrowserActionRef[]):Promise<BrowserPairReady[]>;
export class PairActions {
 constructor(api:unknown,epoch:string);
 execute(op:BrowserPairOperation,paused?:boolean):Promise<BrowserOperationResult|{pair_ready:BrowserPairReady}>;
 continue(continuation:BrowserContinuation,paused?:boolean):Promise<BrowserOperationResult>;
}
