<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { apiSend, errorMessage, refreshSession } from '../api';
const route = useRoute(), router = useRouter();
const form = reactive({ email: String(route.query.email || ''), code: '' });
const loading = ref(false), error = ref('');
async function submit() { error.value=''; if(!form.email.trim()||!form.code.trim()){ error.value='Email and verification code are required.'; return;} loading.value=true; try { await apiSend('/api/verify','POST',form); await refreshSession(); router.push('/onboarding'); } catch(err){ error.value=errorMessage(err);} finally{ loading.value=false; } }
</script>
<template><section class="hero"><div><p class="pill">Verify email</p><h1 class="text-4xl">Enter the one-time code.</h1><p class="text-muted">Verification is handled by the server and starts your authenticated session after success.</p></div><form class="card" @submit.prevent="submit" aria-labelledby="verify-title"><h2 id="verify-title" class="text-2xl">Verification code</h2><p v-if="error" class="alert error" role="alert">{{ error }}</p><div class="field"><label for="verify-email">Email</label><input id="verify-email" v-model="form.email" class="input" type="email" autocomplete="email" required /></div><div class="field"><label for="verify-code">One-time code</label><input id="verify-code" v-model="form.code" class="input" inputmode="numeric" pattern="[0-9]{6}" autocomplete="one-time-code" required /></div><button class="button" :disabled="loading">{{ loading ? 'Verifying…' : 'Verify and continue' }}</button></form></section></template>
