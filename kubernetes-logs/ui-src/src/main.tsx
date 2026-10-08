import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { ThemeProvider, DensityProvider } from '@flanksource/clicky-ui'
import { LogsApp } from './LogsApp'
import { pluginClient } from './api/kubernetesLogs'
import { logBanner } from './version'
import './styles.css'

logBanner()

window.addEventListener('pagehide', event => {
  if (!event.persisted) pluginClient.dispose()
})

const root = document.getElementById('root')
if (!root) throw new Error('missing #root')

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: pluginClient.mode === 'token' ? false : 1,
      staleTime: 30_000,
    },
  },
})

createRoot(root).render(
  <QueryClientProvider client={queryClient}>
    <ThemeProvider>
      <DensityProvider>
        <LogsApp />
      </DensityProvider>
    </ThemeProvider>
  </QueryClientProvider>,
)
