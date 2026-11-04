// Super Sparrow's pages and data are always live and signed-in, so this worker caches nothing.
// It exists so browsers treat the app as installable; navigations go straight to the network and
// every other request, including the live event stream, is left to the browser.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", (event) => {
  if (event.request.mode === "navigate") event.respondWith(fetch(event.request));
});
