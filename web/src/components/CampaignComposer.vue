<script setup lang="ts">
import { computed, ref } from 'vue'
import { useCampaignStore } from '../stores/campaign'
import ContentReview from './ContentReview.vue'

const exampleBrief =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

const store = useCampaignStore()
const brief = ref('')
const canSubmit = computed(() => brief.value.trim() !== '' && !store.generating)

function submit() {
  if (canSubmit.value) store.create(brief.value.trim())
}

const formatCount = (n: number) => n.toLocaleString('tr-TR')
</script>

<template>
  <div class="space-y-6">
    <form data-test="brief-form" class="space-y-3" @submit.prevent="submit">
      <label for="brief" class="block text-sm font-medium text-slate-700">Kampanya talebi</label>
      <textarea
        id="brief"
        v-model="brief"
        data-test="brief"
        rows="4"
        :placeholder="exampleBrief"
        class="w-full rounded-md border border-slate-300 bg-white p-3 text-sm focus:border-slate-500 focus:outline-none"
      />
      <button
        type="submit"
        data-test="submit"
        :disabled="!canSubmit"
        class="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-40"
      >
        {{ store.generating ? 'Oluşturuluyor…' : 'Kampanya oluştur' }}
      </button>
    </form>

    <p v-if="store.error" data-test="campaign-error" class="rounded-md bg-red-50 p-4 text-sm text-red-700">
      {{ store.error }}
    </p>

    <section v-if="store.campaign" class="space-y-4 rounded-lg border border-slate-200 bg-white p-6">
      <h2 class="text-sm font-medium text-slate-500">Hedef kitle</h2>

      <div
        v-if="store.campaign.segment.unsupported_criteria.length"
        data-test="unsupported-warning"
        class="rounded-md bg-amber-50 p-3 text-sm text-amber-800"
      >
        <p class="font-medium">Şu ölçütler hedef kitleye uygulanamadı:</p>
        <ul class="mt-1 list-disc pl-5">
          <li v-for="c in store.campaign.segment.unsupported_criteria" :key="c">{{ c }}</li>
        </ul>
      </div>

      <ul class="flex flex-wrap gap-2">
        <li
          v-for="label in store.campaign.segment.labels"
          :key="label"
          data-test="segment-label"
          class="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-800"
        >
          {{ label }}
        </li>
      </ul>

      <dl class="grid grid-cols-2 gap-4">
        <div class="rounded-md border border-slate-200 p-4">
          <dt class="text-sm text-slate-500">SMS alıcısı</dt>
          <dd data-test="audience-sms" class="mt-1 text-2xl font-semibold">
            {{ formatCount(store.campaign.audience.sms) }}
          </dd>
        </div>
        <div class="rounded-md border border-slate-200 p-4">
          <dt class="text-sm text-slate-500">E-posta alıcısı</dt>
          <dd data-test="audience-email" class="mt-1 text-2xl font-semibold">
            {{ formatCount(store.campaign.audience.email) }}
          </dd>
        </div>
      </dl>
      <p class="text-xs text-slate-500">Sayılar yalnızca o kanal için izin vermiş alıcıları içerir.</p>
    </section>

    <ContentReview v-if="store.campaign" />
  </div>
</template>
