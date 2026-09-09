// Compile a consumer against the same persisted wire fixture used by Go/JS.
import wire from '../../testdata/actions/wire-v1.json' with {type:'json'};
import {validActionRef,actionResult,requestFingerprint,type BrowserActionRef,type BrowserOperationResult,type VerificationState} from '../action-protocol.js';
import {readback,retainedResults,type BrowserReadback} from '../readback.js';
const candidate:unknown=wire.reference;
if(validActionRef(candidate)){
 const ref:BrowserActionRef=candidate;
 const result:BrowserOperationResult=actionResult({action_ref:ref},{operation_id:ref.id,status:'uncertain'});
 requestFingerprint({id:ref.id,epoch:'fixture',action:'open',expires_at:'fixture',action_ref:ref});
 const status:VerificationState='unknown';
 void [result,status];
}
const api:unknown={};
const capture:Promise<BrowserReadback>=readback(api,{id:wire.reference.id},()=>0);
void capture;
void retainedResults(api,validActionRef(candidate)?[candidate]:[]);
// Browser API success is an execution report; it is not a verification value.
// @ts-expect-error independent verification has no succeeded state
const invalidVerification:VerificationState='succeeded';
void invalidVerification;

import {PairActions,retainedPairings,type BrowserContinuation} from '../pairing.js';
if(validActionRef(candidate)){
 const continuation:BrowserContinuation={version:1,id:candidate.id,action_ref:candidate,profile:'fixture',epoch:'fixture',marker_tab_id:7,window_id:3,action:'open',url:'https://example.test/',expires_at:'fixture'};
 const result:Promise<BrowserOperationResult>=new PairActions(api,'fixture').continue(continuation);
 void [result,retainedPairings(api,[candidate])];
}
