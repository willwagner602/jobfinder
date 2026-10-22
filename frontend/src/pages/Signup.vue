<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { apiSend, errorMessage } from '../api';
const router = useRouter();
const form = reactive({ email: '', password: '' });
const loading = ref(false), error = ref(''), success = ref('');
async function submit() {
  error.value = ''; success.value = '';
  if (!form.email.trim()) { error.value = 'Email is required.'; return; }
  loading.value = true;
  try { const result = await apiSend<{ email: string }>('/api/signup', 'POST', { email: form.email, password: form.password || undefined }); success.value = 'Account request received. Check your email for the verification code.'; router.push({ path: '/verify', query: { email: result.email } }); }
  catch (err) { error.value = errorMessage(err); }
  finally { loading.value = false; }
}
</script>
<template>
  <section class="auth-layout">
    <header class="auth-intro"><p class="eyebrow">Create account</p><h1>Create your account</h1><p class="muted">Enter your email and optionally a password. We’ll send a verification code before the account can be used.</p></header>
    <form class="card form auth-form" @submit.prevent="submit" aria-labelledby="signup-title">
      <h2 id="signup-title" class="text-2xl">Sign up</h2><p v-if="error" class="alert error" role="alert">{{ error }}</p><p v-if="success" class="alert success" role="status">{{ success }}</p>
      <div class="field"><label for="signup-email">Email</label><input id="signup-email" v-model="form.email" class="input" type="email" autocomplete="email" required /></div>
      <div class="field"><label for="signup-password">Password <span class="muted">(optional)</span></label><input id="signup-password" v-model="form.password" class="input" type="password" autocomplete="new-password" /></div>
      <div class="form-actions"><button class="button" :disabled="loading">{{ loading ? 'Sending…' : 'Create account and send code' }}</button></div>
    </form>
  </section>
</template>
