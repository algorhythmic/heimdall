import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {validActionRef,executionStates,verificationStates} from '../action-protocol.js';
const fixture=JSON.parse(readFileSync(new URL('../../testdata/actions/wire-v1.json',import.meta.url)));
test('Go and browser shared action wire fixture agrees',()=>{
 assert.deepEqual(executionStates,fixture.execution_states);assert.deepEqual(verificationStates,fixture.verification_states);
 assert(validActionRef(fixture.reference));for(const patch of fixture.reject_patches)assert(!validActionRef({...fixture.reference,...patch}));
 assert(!executionStates.includes('succeeded'));assert(!verificationStates.includes('succeeded'));
});
