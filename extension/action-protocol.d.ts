export type ExecutionState = 'queued' | 'dispatching' | 'api_reported' | 'refused' | 'uncertain' | 'cancelled';
export type VerificationState = 'pending' | 'matched' | 'not_matched' | 'unknown' | 'unsupported';
export interface BrowserActionRef {
  version: 1;
  id: string;
  attempt_id: string;
  intent_digest: string;
  target: string;
  manifest_id: string;
  surface_id: string;
}
export interface BrowserOperationResult {
  operation_id: string;
  continuation_id?: string;
  status: 'succeeded' | 'refused' | 'failed' | 'uncertain';
  tab_id?: number;
  window_id?: number;
  url?: string;
  detail?: string;
  action_ref?: BrowserActionRef;
}
export const executionStates: readonly ExecutionState[];
export const verificationStates: readonly VerificationState[];
export function validActionRef(value: unknown): value is BrowserActionRef;
export function actionResult(op: {action_ref?: BrowserActionRef}, result: BrowserOperationResult): BrowserOperationResult;
export function requestFingerprint(op: {id: string; epoch: string; action: string; expires_at: string; tab_id?: number; window_id?: number; owner_id?: string; expected_url?: string; url?: string; action_ref?: BrowserActionRef; pairing?: import('./pairing.js').BrowserPairingIntent}): string;
