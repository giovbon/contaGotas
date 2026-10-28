// Atualiza a página de detalhe enquanto a extração está em andamento.
// Sem JavaScript, o usuário ainda pode recarregar manualmente.
(() => {
	const alvo = document.querySelector("[data-status-url]");
	if (!alvo || alvo.dataset.status !== "processing") {
		return;
	}

	let restantes = 120; // 3 minutos
	const temporizador = setInterval(async () => {
		if (--restantes < 0) {
			clearInterval(temporizador);
			return;
		}
		try {
			const resposta = await fetch(alvo.dataset.statusUrl, {
				credentials: "same-origin",
				cache: "no-store",
				headers: { Accept: "text/html" },
			});
			if (!resposta.ok) {
				return;
			}
			const html = await resposta.text();
			const novo = new DOMParser().parseFromString(html, "text/html").querySelector("[data-status]");
			if (novo && novo.dataset.status !== alvo.dataset.status) {
				clearInterval(temporizador);
				window.location.reload();
			}
		} catch (_) {
			// Rede instável: tenta de novo no próximo ciclo.
		}
	}, 1500);
})();
