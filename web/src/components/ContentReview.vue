<script setup lang="ts">
import { computed } from 'vue'
import type { Channel, Validation } from '../api'
import { useCampaignStore } from '../stores/campaign'

const store = useCampaignStore()

const channelNames: Record<Channel, string> = { sms: 'SMS', email: 'E-posta' }

const enabled = (c: Channel) => store.enabledChannels.includes(c)

function toggle(c: Channel, e: Event) {
  store.toggleChannel(c, (e.target as HTMLInputElement).checked)
}

const sms = computed(() => store.content.sms)
const email = computed(() => store.content.email)
const smsValidation = computed<Validation | undefined>(() => store.validation.sms)
const emailValidation = computed<Validation | undefined>(() => store.validation.email)

// The backend renders the preview with these sample values.
const sampleNote = 'Örnek değerlerle: {{first_name}} = Ayşe, {{city}} = İstanbul'

const fieldNames: Record<string, string> = {
  body: 'Metin',
  subject: 'Konu',
  preheader: 'Ön izleme metni',
  cta_text: 'Buton metni',
}
</script>

<template>
  <section class="space-y-4 rounded-lg border border-slate-200 bg-white p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-sm font-medium text-slate-500">İçerik</h2>
      <span v-if="store.streaming" data-test="content-streaming" class="text-xs text-slate-500">
        Claude yazıyor…
      </span>
    </div>

    <fieldset class="flex flex-wrap gap-4">
      <legend class="sr-only">Kanallar</legend>
      <label v-for="c in (['sms', 'email'] as Channel[])" :key="c" class="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          :data-test="`channel-${c}`"
          :checked="enabled(c)"
          :disabled="store.streaming"
          @change="toggle(c, $event)"
        />
        {{ channelNames[c] }}
      </label>
    </fieldset>

    <p v-if="store.contentError" data-test="content-error" class="rounded-md bg-red-50 p-4 text-sm text-red-700">
      {{ store.contentError }}
    </p>

    <div class="grid gap-4 md:grid-cols-2">
      <article class="space-y-3 rounded-md border border-slate-200 p-4" :class="{ 'opacity-50': !enabled('sms') }">
        <h3 class="text-sm font-medium">SMS</h3>
        <p v-if="sms" data-test="sms-body" class="whitespace-pre-wrap text-sm">{{ sms.body }}</p>
        <p v-else class="text-sm text-slate-400">{{ store.streaming ? 'Bekleniyor…' : 'Bu kanal için içerik yok.' }}</p>
        <template v-if="smsValidation">
          <p v-if="smsValidation.valid" data-test="sms-valid" class="text-xs text-emerald-700">Kurallara uygun</p>
          <ul v-else data-test="sms-issues" class="list-disc space-y-1 pl-5 text-xs text-red-700">
            <li v-for="(i, n) in smsValidation.issues" :key="n">{{ i.message }}</li>
          </ul>
        </template>
      </article>

      <article class="space-y-3 rounded-md border border-slate-200 p-4" :class="{ 'opacity-50': !enabled('email') }">
        <h3 class="text-sm font-medium">E-posta</h3>
        <dl v-if="email" class="space-y-2 text-sm">
          <div>
            <dt class="text-xs text-slate-500">Konu</dt>
            <dd data-test="email-subject">{{ email.subject }}</dd>
          </div>
          <div>
            <dt class="text-xs text-slate-500">Ön izleme metni</dt>
            <dd data-test="email-preheader">{{ email.preheader }}</dd>
          </div>
          <div>
            <dt class="text-xs text-slate-500">Gövde</dt>
            <dd data-test="email-body" class="whitespace-pre-wrap">{{ email.body }}</dd>
          </div>
          <div v-if="email.cta_text">
            <dt class="text-xs text-slate-500">Buton</dt>
            <dd data-test="email-cta">{{ email.cta_text }}</dd>
          </div>
        </dl>
        <p v-else class="text-sm text-slate-400">{{ store.streaming ? 'Bekleniyor…' : 'Bu kanal için içerik yok.' }}</p>
        <template v-if="emailValidation">
          <p v-if="emailValidation.valid" data-test="email-valid" class="text-xs text-emerald-700">Kurallara uygun</p>
          <ul v-else data-test="email-issues" class="list-disc space-y-1 pl-5 text-xs text-red-700">
            <li v-for="(i, n) in emailValidation.issues" :key="n">
              {{ fieldNames[i.field] ?? i.field }}: {{ i.message }}
            </li>
          </ul>
        </template>
      </article>
    </div>

    <div v-if="store.emailPreview" class="space-y-2">
      <h3 class="text-sm font-medium">E-posta önizlemesi</h3>
      <p class="text-xs text-slate-500">{{ sampleNote }}</p>
      <iframe
        data-test="email-preview"
        title="E-posta önizlemesi"
        sandbox=""
        :srcdoc="store.emailPreview"
        class="h-[28rem] w-full rounded-md border border-slate-200"
      />
    </div>
  </section>
</template>
