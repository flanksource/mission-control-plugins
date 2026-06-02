export function getConfigId() {
  const params = new URLSearchParams(location.search)
  const configId = params.get('config_id')
  if (configId) return configId

  try {
    return new URL(document.referrer).searchParams.get('config_id') ?? ''
  } catch {
    return ''
  }
}
