<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import {
  ApiError,
  cancelCampaign,
  cancellable,
  channels,
  subscribeDashboard,
  type CampaignProgress,
  type ChannelCounts,
} from '../api'

const campaigns = ref<CampaignProgress[]>([])
const cancelling = ref<Record<number, boolean>>({})
const cancelErrors = ref<Record<number, string>>({})

// cancel stops the Campaign. The stream confirms the status on its next
// event; until then the row shows it cancelled right away.
async function cancel(c: CampaignProgress) {
  cancelling.value[c.id] = true
  delete cancelErrors.value[c.id]
  try {
    c.status = await cancelCampaign(c.id)
  } catch (e) {
    cancelErrors.value[c.id] = e instanceof ApiError ? e.message : 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.'
  } finally {
    delete cancelling.value[c.id]
  }
}
let close: (() => void) | null = null

onMounted(() => {
  close = subscribeDashboard((e) => (campaigns.value = e.campaigns))
})
onUnmounted(() => close?.())

const statusNames: Record<string, string> = {
  approved: 'Onaylandı',
  sending: 'Gönderiliyor',
  completed: 'Tamamlandı',
  cancelled: 'İptal edildi',
}
const channelNames = { sms: 'SMS', email: 'E-posta' }

const percent = (c: CampaignProgress) => Math.round(c.progress * 100)
const channelPercent = (c: ChannelCounts) => (c.total ? Math.round(((c.sent + c.failed) / c.total) * 100) : 0)
const formatCount = (n: number) => n.toLocaleString('tr-TR')
const sentOn = (c: CampaignProgress) =>
  channels.flatMap((ch) => (c.deliveries[ch] ? [{ channel: ch, counts: c.deliveries[ch] }] : []))
</script>

<template>
  <section v-if="campaigns.length" data-test="dashboard" class="space-y-4 rounded-lg border border-slate-200 bg-white p-6">
    <h2 class="text-sm font-medium text-slate-500">Gönderim durumu</h2>
    <article
      v-for="c in campaigns"
      :key="c.id"
      :data-test="`dashboard-campaign-${c.id}`"
      class="space-y-3 border-t border-slate-100 pt-4 first-of-type:border-t-0 first-of-type:pt-0"
    >
      <div class="flex items-baseline justify-between gap-4">
        <p class="truncate text-sm text-slate-700" :title="c.brief">#{{ c.id }} · {{ c.brief }}</p>
        <div class="flex shrink-0 items-center gap-2">
          <span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
            {{ statusNames[c.status] ?? c.status }}
          </span>
          <button
            v-if="cancellable.includes(c.status)"
            type="button"
            data-test="cancel"
            :disabled="cancelling[c.id]"
            class="rounded-md border border-red-200 px-2 py-0.5 text-xs font-medium text-red-700 hover:bg-red-50 disabled:opacity-40"
            @click="cancel(c)"
          >
            {{ cancelling[c.id] ? 'İptal ediliyor…' : 'İptal et' }}
          </button>
        </div>
      </div>
      <p v-if="cancelErrors[c.id]" data-test="cancel-error" class="rounded-md bg-red-50 p-2 text-sm text-red-700">
        {{ cancelErrors[c.id] }}
      </p>
      <div class="flex items-center gap-3">
        <div
          data-test="progress"
          role="progressbar"
          aria-label="Gönderim ilerlemesi"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-valuenow="percent(c)"
          class="h-2 flex-1 overflow-hidden rounded-full bg-slate-100"
        >
          <div class="h-full bg-emerald-600 transition-all" :style="{ width: `${percent(c)}%` }" />
        </div>
        <span class="w-10 text-right text-xs tabular-nums text-slate-500">%{{ percent(c) }}</span>
      </div>
      <dl class="grid gap-2 text-sm sm:grid-cols-2">
        <div v-for="d in sentOn(c)" :key="d.channel" :data-test="`counts-${d.channel}`" class="rounded-md bg-slate-50 p-3">
          <dt class="font-medium text-slate-700">{{ channelNames[d.channel] }} · %{{ channelPercent(d.counts) }}</dt>
          <dd class="mt-1 flex flex-wrap gap-x-4 tabular-nums text-slate-600">
            <span>Bekleyen {{ formatCount(d.counts.pending) }}</span>
            <span class="text-emerald-700">Gönderildi {{ formatCount(d.counts.sent) }}</span>
            <span class="text-red-700">Başarısız {{ formatCount(d.counts.failed) }}</span>
          </dd>
        </div>
      </dl>
    </article>
  </section>
</template>
