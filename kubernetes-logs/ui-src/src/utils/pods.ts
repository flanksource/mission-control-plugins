import type { PodRow, SelectedPod } from '../types'

export function parseSelectedPod(value: string): SelectedPod | null {
  if (!value) return null
  const [namespace, pod] = value.split('|')
  if (!namespace || !pod) return null
  return { namespace, pod }
}

export function selectedPodValue(pod: Pick<PodRow, 'namespace' | 'pod'>) {
  return `${pod.namespace}|${pod.pod}`
}
