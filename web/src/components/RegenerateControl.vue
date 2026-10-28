<script setup lang="ts">
import { ref } from 'vue'

// RegenerateControl is the "Claude ile yeniden üret" action for one part of
// the proposal (the Segment Rule or one Channel's Content), with an optional
// free-text instruction.
const props = defineProps<{ part: string; placeholder: string; busy: boolean; disabled: boolean }>()
const emit = defineEmits<{ regenerate: [instruction: string] }>()

const instruction = ref('')

function submit() {
  if (!props.disabled) emit('regenerate', instruction.value.trim())
}
</script>

<template>
  <form class="flex flex-wrap items-center gap-2" @submit.prevent="submit">
    <label :for="`${part}-instruction`" class="sr-only">Yeniden üretim talimatı</label>
    <input
      :id="`${part}-instruction`"
      v-model="instruction"
      :data-test="`${part}-instruction`"
      type="text"
      :placeholder="placeholder"
      :disabled="disabled"
      class="min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none disabled:bg-slate-50"
    />
    <button
      type="button"
      :data-test="`${part}-regenerate`"
      :disabled="disabled"
      class="rounded-md border border-slate-300 bg-white px-3 py-1 text-sm font-medium text-slate-800 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
      @click="submit"
    >
      {{ busy ? 'Yeniden üretiliyor…' : 'Claude ile yeniden üret' }}
    </button>
    <span v-if="busy" :data-test="`${part}-regenerating`" role="status" class="text-xs text-slate-500">
      Claude yeniden yazıyor…
    </span>
  </form>
</template>
