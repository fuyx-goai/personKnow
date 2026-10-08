<script setup>
import { onMounted } from 'vue'
import Masthead from './components/Masthead.vue'
import SpineNav from './components/SpineNav.vue'
import ToastHost from './components/ToastHost.vue'
import { bootstrap } from './store'

onMounted(bootstrap)
</script>

<template>
  <SpineNav />
  <main class="stage">
    <Masthead />
    <RouterView v-slot="{ Component }">
      <Transition name="view" mode="out-in">
        <component :is="Component" />
      </Transition>
    </RouterView>
  </main>
  <ToastHost />
</template>
