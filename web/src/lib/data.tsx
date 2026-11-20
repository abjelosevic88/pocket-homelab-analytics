// The person's websites, shared by every page (the sidebar shows them with
// the people on each right now).
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { Site } from './types'

const EVENT = 'pan:changed'

interface DataState {
  sites: Site[] | null
  geo: string
  reload: () => Promise<void>
}
const Ctx = createContext<DataState>({ sites: null, geo: '', reload: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (a site added, a goal saved): lists reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [sites, setSites] = useState<Site[] | null>(null)
  const [geo, setGeo] = useState('')
  const reload = useCallback(async () => {
    try {
      const r = await api.get<{ sites: Site[]; geo: string }>(`${V1}/sites`)
      setSites(r.sites); setGeo(r.geo)
    } catch { setSites(x => x || []) }
  }, [])
  useEffect(() => { reload() }, [reload])
  useOnChange(reload)
  // the people on each site now: look once a minute, quietly
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') reload() }, 60000)
    return () => window.clearInterval(t)
  }, [reload])
  return <Ctx.Provider value={{ sites, geo, reload }}>{children}</Ctx.Provider>
}
