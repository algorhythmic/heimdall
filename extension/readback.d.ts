import type {BrowserActionRef,BrowserOperationResult} from './action-protocol.js';
export interface BrowserTabReadback {id:number;window_id:number;url:string;title:string;active:boolean;load_status?:''|'unloaded'|'loading'|'complete';discarded?:boolean;navigation_pending?:boolean}
export interface BrowserChallenge {version:1;id:string;profile:string;epoch:string;connection:string;runtime_id:string;after_event_id:number;issued_at:string;expires_at:string;actions:BrowserActionRef[]}
export interface BrowserReadback {event_generation:number;markers:import('./pairing.js').BrowserMarker[];type:'readback';challenge_id:string;observed_at:string;tabs:BrowserTabReadback[];present_tabs:number[];instances:{tab_id:number;action_ref:BrowserActionRef}[];focused_window:number;complete:boolean;stable:boolean}
export function retainedResults(api:unknown,refs:BrowserActionRef[]):Promise<BrowserOperationResult[]>;
export function readback(api:unknown,challenge:Pick<BrowserChallenge,'id'>,generation?:()=>number):Promise<BrowserReadback>;
