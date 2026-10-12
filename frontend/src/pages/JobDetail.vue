<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RouterLink, useRoute } from 'vue-router';
import { ExternalLink } from 'lucide-vue-next';
import { apiGet, errorMessage, Job } from '../api';
const route = useRoute();
const loading = ref(true), error = ref(''), job = ref<Job | null>(null);
onMounted(async()=>{ try{ const data=await apiGet<{job:Job}>(`/api/jobs/${route.params.matchId}`); job.value=data.job; } catch(err){ error.value=errorMessage(err);} finally{ loading.value=false; } });
</script>
<template><section><RouterLink class="nav-link" to="/jobs">← Back to job matches</RouterLink><p v-if="loading" class="card mt-4" role="status">Loading job match…</p><p v-else-if="error" class="alert error" role="alert">{{ error }}</p><article v-else-if="job" class="card mt-4"><p class="pill">{{ job.decision }}</p><h1 class="text-4xl">{{ job.title }}</h1><p class="meta"><span><strong>Company:</strong> {{ job.company }}</span><span v-if="job.rating"><strong>Rating:</strong> {{ job.rating }}/5</span></p><p v-if="job.canonicalUrl" class="mt-4"><a class="button secondary" :href="job.canonicalUrl" rel="noreferrer" target="_blank"><ExternalLink class="h-4 w-4" aria-hidden="true" /> Original listing</a></p><section class="mt-4"><h2>Summary</h2><pre>{{ job.summary || 'No summary was returned for this match.' }}</pre></section></article></section></template>
