<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RouterLink, useRoute } from 'vue-router';
import { ExternalLink } from 'lucide-vue-next';
import { apiGet, errorMessage, Job } from '../api';
const route = useRoute();
const loading = ref(true), error = ref(''), job = ref<Job | null>(null);
onMounted(async () => { try { const data = await apiGet<{ job: Job }>(`/api/jobs/${route.params.matchId}`); job.value = data.job; } catch (err) { error.value = errorMessage(err); } finally { loading.value = false; } });
</script>
<template>
  <section class="page-stack">
    <RouterLink class="nav-link detail-back" to="/jobs">← Back to job matches</RouterLink>
    <p v-if="loading" class="card" role="status">Loading job match…</p><p v-else-if="error" class="alert error" role="alert">{{ error }}</p>
    <article v-else-if="job" class="card detail-card">
      <span class="pill job-decision">{{ job.decision }}</span><h1>{{ job.title }}</h1>
      <p class="meta"><span><strong>Company</strong> {{ job.company }}</span><span v-if="job.rating"><strong>Rating</strong> {{ job.rating }}/5</span></p>
      <p v-if="job.canonicalUrl"><a class="button secondary" :href="job.canonicalUrl" rel="noreferrer" target="_blank"><ExternalLink class="h-4 w-4" aria-hidden="true" /> Original listing</a></p>
      <section class="detail-summary"><h2>Summary</h2><p>{{ job.summary || 'No summary was returned for this match.' }}</p></section>
    </article>
  </section>
</template>
