<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { apiSend, errorMessage, refreshSession } from '../api';
const route = useRoute(), router = useRouter();
const form = reactive({ email: '', code: '' });
const loading = ref(false), error = ref('');
onMounted(() => { form.email = String(route.query.email || ''); });
async function submit() {
  error.value = '';
  if (!form.email.trim() || !form.code.trim()) { error.value = 'Email and verification code are required.'; return; }
  loading.value = true;
  try { await apiSend('/api/verify', 'POST', form); await refreshSession(); router.push('/onboarding'); }
  catch (err) { error.value = errorMessage(err); }
  finally { loading.value = false; }
}
</script>
<template>
  <section class="auth-layout">
    <header class="auth-intro"><p class="eyebrow">Verify email</p><h1>Enter your code</h1><p class="muted">Verification is handled securely and starts your session after success.</p></header>
    <form class="card form auth-form" @submit.prevent="submit" aria-labelledby="verify-title">
      <h2 id="verify-title" class="text-2xl">Verification code</h2><p v-if="error" class="alert error" role="alert">{{ error }}</p>
      <div class="field"><label for="verify-email">Email</label><input id="verify-email" v-model="form.email" class="input" type="email" autocomplete="email" required /></div>
      <div class="field"><label for="verify-code">One-time code</label><input id="verify-code" v-model="form.code" class="input" inputmode="numeric" autocomplete="one-time-code" required /></div>
      <div class="form-actions"><button class="button" :disabled="loading">{{ loading ? 'Verifying…' : 'Verify and continue' }}</button></div>
    </form>
  </section>
</template>
