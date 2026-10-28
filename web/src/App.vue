<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useOverviewStore } from './stores/overview'

const store = useOverviewStore()
onMounted(() => store.load())

const recipientCount = computed(() =>
  store.overview ? store.overview.recipient_count.toLocaleString('tr-TR') : null,
)
</script>

<template>
  <div class="min-h-screen bg-slate-50 text-slate-900">
    <header class="border-b border-slate-200 bg-white">
      <div class="mx-auto flex max-w-5xl items-center justify-between px-4 py-4">
        <h1 class="text-lg font-semibold">Akıllı Kampanya Oluşturucu</h1>
        <span
          v-if="store.overview?.llm.mock"
          data-test="mock-llm-badge"
          title="ANTHROPIC_API_KEY tanımlı değil; Claude yerine senaryolu sahte model kullanılıyor."
          class="rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-800"
          >Mock LLM</span
        >
      </div>
    </header>

    <main class="mx-auto max-w-5xl px-4 py-8">
      <p v-if="store.failed" data-test="overview-error" class="rounded-md bg-red-50 p-4 text-red-700">
        Sunucuya ulaşılamadı. Lütfen daha sonra tekrar deneyin.
      </p>
      <section v-else class="w-full max-w-xs rounded-lg border border-slate-200 bg-white p-6">
        <h2 class="text-sm text-slate-500">Toplam alıcı</h2>
        <p v-if="recipientCount" data-test="recipient-count" class="mt-1 text-3xl font-semibold">
          {{ recipientCount }}
        </p>
        <p v-else class="mt-1 text-slate-400">Yükleniyor…</p>
      </section>
    </main>
  </div>
</template>
