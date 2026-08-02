import Vue from "vue";
import Vuex from "vuex";

Vue.use(Vuex);

export default new Vuex.Store({
  state: {
    admin: null
  },
  mutations: {
    setAdmin(state, admin) {
      state.admin = admin;
    },
    setCSRFToken(state, token) {
      if (token) {
        sessionStorage.setItem("control_csrf_token", token);
      } else {
        sessionStorage.removeItem("control_csrf_token");
      }
    }
  }
});
