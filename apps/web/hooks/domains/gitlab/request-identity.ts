export function isCurrentIdentityRequest(
  requestGeneration: number,
  currentGeneration: number,
  requestIdentity: string,
  currentIdentity: string,
): boolean {
  return requestGeneration === currentGeneration && requestIdentity === currentIdentity;
}
