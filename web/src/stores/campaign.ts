import { defineStore } from 'pinia'
import { ApiError, createCampaign, type Campaign } from '../api'

export const useCampaignStore = defineStore('campaign', {
  state: () => ({
    campaign: null as Campaign | null,
    generating: false,
    error: null as string | null,
  }),
  actions: {
    async create(brief: string) {
      this.generating = true
      this.error = null
      this.campaign = null
      try {
        this.campaign = await createCampaign(brief)
      } catch (e) {
        this.error = e instanceof ApiError ? e.message : 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.'
      } finally {
        this.generating = false
      }
    },
  },
})
