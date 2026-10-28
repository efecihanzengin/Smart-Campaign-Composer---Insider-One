<script setup lang="ts">
import { computed } from 'vue'
import type { Channel, EmailContent, Length, Validation } from '../api'
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

// The counters show what the backend validator measured; no limits are
// computed here (ADR-0002).
const smsUsage = computed<Length | undefined>(() => smsValidation.value?.usage?.body)
const emailUsage = computed(() => emailValidation.value?.usage)

const encodingNames = { gsm7: 'GSM-7', ucs2: 'UCS-2' }
const ucs2Hint = 'ş, ğ, ı, İ gibi harfler SMS sınırını 160 yerine 70 karaktere düşürür.'

const counterClass = (l: Length) => (l.used > l.limit ? 'font-medium text-red-700' : 'text-slate-500')

const emailFields: { field: keyof EmailContent; test: string; label: string; multiline?: boolean }[] = [
  { field: 'subject', test: 'subject', label: 'Konu' },
  { field: 'preheader', test: 'preheader', label: 'Ön izleme metni' },
  { field: 'body', test: 'body', label: 'Gövde', multiline: true },
  { field: 'cta_text', test: 'cta', label: 'Buton metni (isteğe bağlı)' },
]

const inputValue = (e: Event) => (e.target as HTMLInputElement | HTMLTextAreaElement).value

const inputClass =
  'w-full rounded-md border border-slate-300 bg-white p-2 text-sm focus:border-slate-500 focus:outline-none disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-500'

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
      <span
        v-if="store.repairing"
        data-test="content-repairing"
        role="status"
        class="rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-800"
      >
        Kurallara uymayan içerik düzeltiliyor ({{ store.repairing.repair }}/{{ store.repairing.max }})…
      </span>
      <span v-else-if="store.streaming" data-test="content-streaming" class="text-xs text-slate-500">
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
          :disabled="store.locked || !!store.disabledChannels[c]"
          @change="toggle(c, $event)"
        />
        {{ channelNames[c] }}
      </label>
    </fieldset>
    <p
      v-for="(why, c) in store.disabledChannels"
      :key="c"
      :data-test="`channel-${c}-disabled`"
      class="rounded-md bg-amber-50 p-3 text-sm text-amber-800"
    >
      {{ why }}
    </p>

    <div
      v-if="store.contentError"
      data-test="content-error"
      class="flex flex-wrap items-center justify-between gap-3 rounded-md bg-red-50 p-4 text-sm text-red-700"
    >
      <p>{{ store.contentError }}</p>
      <button
        v-if="store.contentRetryable"
        type="button"
        data-test="content-retry"
        :disabled="store.streaming"
        class="rounded-md border border-red-300 bg-white px-3 py-1 font-medium text-red-700 hover:bg-red-100 disabled:opacity-40"
        @click="store.retry()"
      >
        Tekrar dene
      </button>
    </div>

    <div class="grid gap-4 md:grid-cols-2">
      <article class="space-y-3 rounded-md border border-slate-200 p-4" :class="{ 'opacity-50': !enabled('sms') }">
        <h3 class="text-sm font-medium">SMS</h3>
        <div v-if="sms" class="space-y-1">
          <label for="sms-body" class="sr-only">SMS metni</label>
          <textarea
            id="sms-body"
            data-test="sms-body"
            rows="5"
            :value="sms.body"
            :disabled="store.locked"
            :class="inputClass"
            @input="store.edit('sms', { body: inputValue($event) })"
            @change="store.save('sms')"
          />
          <p
            v-if="smsUsage"
            data-test="sms-counter"
            class="text-right text-xs tabular-nums"
            :class="counterClass(smsUsage)"
            :title="smsUsage.encoding === 'ucs2' ? ucs2Hint : undefined"
          >
            {{ encodingNames[smsUsage.encoding ?? 'gsm7'] }} · {{ smsUsage.used }}/{{ smsUsage.limit }}
          </p>
        </div>
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
        <div v-if="email" class="space-y-3 text-sm">
          <div v-for="f in emailFields" :key="f.field" class="space-y-1">
            <div class="flex items-baseline justify-between gap-2">
              <label :for="`email-${f.test}`" class="text-xs text-slate-500">{{ f.label }}</label>
              <span
                v-if="emailUsage?.[f.field]"
                :data-test="`email-${f.test}-counter`"
                class="text-xs tabular-nums"
                :class="counterClass(emailUsage[f.field])"
              >
                {{ emailUsage[f.field].used }}/{{ emailUsage[f.field].limit }}
              </span>
            </div>
            <component
              :is="f.multiline ? 'textarea' : 'input'"
              :id="`email-${f.test}`"
              :data-test="`email-${f.test}`"
              :rows="f.multiline ? 8 : undefined"
              :value="email[f.field] ?? ''"
              :disabled="store.locked"
              :class="inputClass"
              @input="store.edit('email', { [f.field]: inputValue($event) })"
              @change="store.save('email')"
            />
          </div>
        </div>
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
