<script setup lang="ts">
import { computed } from 'vue'
import { useCampaignStore } from '../stores/campaign'

const store = useCampaignStore()

const blocked = computed(() => store.approvalBlockers.length > 0 || store.approving)

const formatCount = (n: number) => n.toLocaleString('tr-TR')

const summary = computed(() => {
  const d = store.approval?.deliveries ?? {}
  const parts: string[] = []
  if (d.sms) parts.push(`${formatCount(d.sms)} SMS`)
  if (d.email) parts.push(`${formatCount(d.email)} e-posta`)
  return parts.join(' ve ')
})
</script>

<template>
  <section class="space-y-3 rounded-lg border border-slate-200 bg-white p-6">
    <h2 class="text-sm font-medium text-slate-500">Onay</h2>

    <p v-if="store.approval" data-test="approved" class="rounded-md bg-emerald-50 p-4 text-sm text-emerald-800">
      Kampanya onaylandı. Hedef kitle donduruldu: {{ summary }} gönderimi oluşturuldu.
    </p>

    <p
      v-else-if="store.campaign?.status === 'cancelled'"
      data-test="cancelled"
      class="rounded-md bg-slate-100 p-4 text-sm text-slate-700"
    >
      Kampanya iptal edildi; hiçbir mesaj gönderilmeyecek.
    </p>

    <template v-else>
      <p class="text-sm text-slate-600">
        Onayladığınızda hedef kitle o anki haliyle dondurulur; izinler bir kez daha kontrol edilir.
      </p>
      <ul v-if="store.approvalBlockers.length" class="list-disc space-y-1 pl-5 text-sm text-amber-800">
        <li v-for="r in store.approvalBlockers" :key="r" data-test="approve-reason">{{ r }}</li>
      </ul>
      <p v-if="store.approvalError" data-test="approve-error" class="rounded-md bg-red-50 p-4 text-sm text-red-700">
        {{ store.approvalError }}
      </p>
      <button
        type="button"
        data-test="approve"
        :disabled="blocked"
        class="rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-40"
        @click="store.approve()"
      >
        {{ store.approving ? 'Onaylanıyor…' : 'Kampanyayı onayla' }}
      </button>
      <button
        v-if="store.campaign?.status === 'draft'"
        type="button"
        data-test="cancel-draft"
        :disabled="store.cancelling || store.approving"
        class="ml-3 rounded-md border border-red-200 px-4 py-2 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
        @click="store.cancelDraft()"
      >
        {{ store.cancelling ? 'İptal ediliyor…' : 'Kampanyayı iptal et' }}
      </button>
    </template>
  </section>
</template>
