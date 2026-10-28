import { defineStore } from 'pinia'
import { fetchOverview, type Overview } from '../api'

export const useOverviewStore = defineStore('overview', {
  state: () => ({
    overview: null as Overview | null,
    failed: false,
  }),
  actions: {
    async load() {
      try {
        this.overview = await fetchOverview()
        this.failed = false
      } catch {
        this.failed = true
      }
    },
  },
})
