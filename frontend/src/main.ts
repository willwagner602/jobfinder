import { createApp } from 'vue';
import { createRouter, createWebHistory } from 'vue-router';
import App from './App.vue';
import Dashboard from './pages/Dashboard.vue';
import Signup from './pages/Signup.vue';
import Verify from './pages/Verify.vue';
import Login from './pages/Login.vue';
import Profile from './pages/Profile.vue';
import JobsList from './pages/JobsList.vue';
import JobDetail from './pages/JobDetail.vue';
import './styles.css';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/signup', component: Signup },
    { path: '/verify', component: Verify },
    { path: '/login', component: Login },
    { path: '/onboarding', component: Profile },
    { path: '/profile', redirect: '/onboarding' },
    { path: '/jobs', component: JobsList },
    { path: '/jobs/:matchId', component: JobDetail },
  ],
});

createApp(App).use(router).mount('#app');
