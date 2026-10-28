<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { apiSend, errorMessage, refreshSession } from '../api';
const route = useRoute(), router = useRouter();
const form = reactive({ email: '', code: '' });
const loading = ref(false), error = ref('');
const emailKnown = computed(() => form.email.trim() !== '');
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
    <header class="auth-intro"><p class="eyebrow">Verify email</p><h1>Verification code</h1><p class="muted">Enter the code from your email to finish creating your account.</p></header>
    <form class="card form auth-form" @submit.prevent="submit" aria-labelledby="verify-title">
      <h2 id="verify-title" class="visually-hidden">Verification code</h2><p v-if="error" class="alert error" role="alert">{{ error }}</p>
      <p v-if="emailKnown" class="muted" data-testid="verification-email-notice">We've sent an email to <strong>{{ form.email }}</strong></p>
      <div v-else class="field"><label for="verify-email">Email</label><input id="verify-email" v-model="form.email" class="input" type="email" autocomplete="email" required /></div>
      <div class="field"><label for="verify-code">One-time code</label><input id="verify-code" v-model="form.code" class="input" inputmode="numeric" autocomplete="one-time-code" required /></div>
      <div class="form-actions"><button class="button" :disabled="loading">{{ loading ? 'Verifying…' : 'Verify and continue' }}</button></div>
    </form>
  </section>
</template>
