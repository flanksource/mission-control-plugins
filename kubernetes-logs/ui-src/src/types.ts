export type PodRow = {
  namespace: string
  pod: string
  phase: string
  ownedBy?: string
}

export type SelectedPod = {
  namespace: string
  pod: string
}
