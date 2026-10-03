# Revisión 03 — Arquitecturas de recuperación y memoria que pueden inspirar la próxima versión

*3 de octubre de 2026. Revisión de literatura; no se corrió nada ni se gastó JEV/Gemini. Base: `plan.md` §17–29, `internal/explorer/explorer.go`, `internal/notebook/judge.go`.*

**Cómo leer las cifras.** arxiv.org y aclanthology.org están bloqueados desde aquí. Cada número lleva su origen:
**[HF]** = página del artículo en huggingface.co/papers (resumen del propio artículo); **[GH]** = README del repositorio; **[blog]** = página oficial del autor; **[web]** = fragmento de buscador o fuente secundaria, **no verificado contra el PDF**; **[plan]** = ya citado en `plan.md`. Donde no encontré cifra, lo digo. Los escenarios de cada artículo son distintos al nuestro: comparar ganancias relativas, nunca cifras absolutas.

---

## (a) Resumen en 10 líneas

1. Casi toda la literatura multi-salto usa un **generador por paso** (IRCoT, Self-Ask, ReAct, FLARE, CoRAG, Search-R1) para escribir la siguiente consulta; JEV no puede, pero nuestro cuaderno ya sustituye esa consulta escrita por **frases textuales escogidas por JEV**, y eso da ganancias del mismo orden.
2. La pieza más cercana a lo nuestro es **BridgeRAG (2026)**: puntúa s(pregunta, puente, candidato) y logra R@5 0.815 en MuSiQue sin entrenar; su juez es un LLM, pero esa *decisión* es exactamente un `noul` de JEV. Confirma la idea del cuaderno.
3. **PropRAG** (R@5 0.783, sin LLM en consulta) y **EfficientRAG** (sin LLM por iteración) muestran que el bucle puede ser barato si el trabajo generativo se mueve a la **ingesta** o a modelos pequeños.
4. Los grafos (HippoRAG 2, CatRAG, LinearRAG) suben poco el R@5 pero sí la **cadena completa**; CatRAG confirma que el peso de una conexión debe depender de la pregunta. Nuestro grafo estático con PPR no ganó (§19, §21): sirve como *canal de candidatos condicionado al cuaderno*, no como ordenador.
5. Los reordenadores estándar dañan puentes (lo vimos en §27–28); los que no dañan son **condicionados al conjunto** (DualView, DPS, utilidad contextual, AAR) y todos requieren entrenamiento.
6. En abstención, el consenso 2025–26 (Sufficient Context, SURE-RAG, RefusalBench) es que la suficiencia es **propiedad del conjunto** y se decide mejor con varias señales calibradas combinadas: justo lo que JEV sabe dar.
7. Lo generativo que sí vale la pena pagar va **una vez, en la ingesta**: contexto por fragmento (Contextual Retrieval), proposiciones autocontenidas (PropRAG), pseudo-preguntas (HopRAG), resúmenes de árbol (RAPTOR/PageIndex).
8. Hallazgo en el código: las búsquedas de la ronda ≥ 2 usan el texto del hecho **sin el título de su fuente** (`factText`) y concatenan hasta 8 hechos en un solo vector; ambas cosas diluyen la consulta del puente. Arreglarlo cuesta cero JEV.
9. Top 5 (sección c): consultas por hecho con título; abstención por regresión sobre señales ya calculadas; canal de entidades guiado por el cuaderno; parada adaptativa con rondas extra para preguntas difíciles; enriquecimiento en ingesta.
10. Tres de las cinco se prueban **sin gastar JEV** (con trazas y caché existentes); las otras dos cuestan ≈ US$0.02–0.08. Ningún cambio debe tocar la configuración congelada de H9 (§29) hasta que esa corrida termine.

---

## (b) Tabla de arquitecturas

Leyenda de la columna JEV: **✔ decide** = la pieza es una decisión cerrada que JEV puede tomar; **✎ gen-consulta** = necesita escribir texto en cada consulta (JEV no puede); **✎ gen-ingesta** = necesita texto, pero se puede generar una sola vez al indexar con un LLM pequeño.

### B1. Bucles iterativos y agentes

| Arquitectura | Idea central | Ganancia reportada | Costo | Qué hace JEV / qué necesita generador | Boceto en el explorador |
|---|---|---|---|---|---|
| **IRCoT** (2022) | Alterna un paso de cadena de pensamiento con una búsqueda que usa esa frase como consulta. | Recall: Flan-T5-XXL +7.9 HotpotQA, +14.3 2Wiki, **+3.5 MuSiQue**, +10.2 IIRC; GPT-3 +11.3 / +22.6 / **+12.5** / +21.2 [HF] | Una generación LLM por paso | ✎ gen-consulta (la frase de razonamiento). ✔ JEV ya hace el equivalente: escoge la frase textual que "razona" el siguiente paso. | Ya implementado en espíritu (cuaderno). Lo que falta de IRCoT: usar **la última frase** como consulta propia, no la concatenación de todo (→ propuesta 1). |
| **Self-Ask** (2022) | El modelo escribe sub-preguntas de seguimiento y las responde con buscador. | Brecha de composicionalidad ~40% constante con el tamaño; Self-Ask +11 pts absolutos sobre CoT en Bamboogle [HF] | Generación por sub-pregunta | ✎ gen-consulta (escribir sub-preguntas). ✔ JEV puede decidir "¿hace falta otra pregunta de seguimiento?" (`noul` sobre el cuaderno). | Sustituto sin generador: cada hecho nuevo *es* la respuesta a una sub-pregunta implícita; buscar con cada hecho por separado (propuesta 1). |
| **ReAct** (2022) | Intercala "pensamiento" y "acción" (buscar, leer, terminar). | HotpotQA EM 27.4 (CoT 29.4); ReAct→CoT-SC 35.1; FEVER 60.9 / 64.6 [HF] | Muchas llamadas LLM por pregunta | ✎ los pensamientos. ✔ la **acción** sí: `choice` entre {buscar con hecho X, seguir leyendo pasaje Y, abrir vecino Z, parar, abstenerse}. | Un `choice` de "siguiente acción" por ronda sobre el estado `{query, cuaderno, opciones}`; las opciones las arma el código. |
| **Search-R1** (2025) y agentes RL | LLM entrenado con RL para decidir cuándo y qué buscar. | +24% (7B) y +20% (3B) relativo sobre RAG en 7 conjuntos [web; otra versión del resumen dice +41%] — no verificado | Entrenar con RL + generación por paso | ✎ gen-consulta. Lo aprovechable: la recompensa por resultado sirve para **ajustar umbrales** de JEV, no para entrenarlo. | No aplicable directamente. |
| **CoRAG** (2025) | Cadenas de recuperación entrenadas (rejection sampling de sub-consultas). | > 10 pts EM en multi-salto sobre bases fuertes [web, no verificado] | Entrenar + generar | ✎ gen-consulta. | No aplicable; útil como referencia de techo. |
| **FLARE** (2023) | Genera la frase siguiente provisional; si tiene tokens de baja probabilidad, busca con ella. | "Superior o competitivo" en 4 tareas, mayor ganancia en multi-salto (2Wiki); sin cifra verificada [HF] | Generación + búsqueda condicional | ✎ la frase provisional. ✔ la **disparadora** (¿falta información?) es una probabilidad calibrada: JEV `missing_link`. | Usar `missing_link` del cuaderno como disparador de "otra ronda" (propuesta 4). |
| **Adaptive-RAG** (2024) | Un clasificador pequeño (T5-large, 770M) decide: sin búsqueda / una búsqueda / multi-paso. | 3.6× más rápido que multi-paso con exactitud competitiva; oráculo de clasificación 56.28 F1 [HF] | Clasificador barato + el método elegido | ✔ totalmente: es un `choice` de 3 opciones o un `noul` "¿basta una pasada?". | Tras la ronda 1, JEV decide si seguir; reasignar el presupuesto ahorrado a preguntas de 3–4 saltos (propuesta 4). |
| **Self-RAG** (2023) | El generador emite fichas de reflexión: Retrieve, IsRel, IsSup, IsUse. | Supera a ChatGPT y Llama2-chat en QA abierta, verificación y biografías; mejor precisión de citas [HF; sin cifra extraída] | Entrenar crítico + generador (7B/13B) | ✔ **las cuatro fichas son decisiones cerradas** (sí/no o 3 niveles): JEV las da sin entrenar. ✎ la respuesta. | Ya tenemos IsRel (`next_needed`) y algo de IsSup (`answer_stated`). Agregar IsSup **por cita** en la salida (fase 4, verificación de citas). |
| **CRAG** (2024) | Evaluador ligero (T5-large 0.77B) clasifica la recuperación en correcta / incorrecta / ambigua y dispara acción. | PopQA +7.0%, Biografía +14.9% FactScore, PubHealth +36.6%, Arc +15.4% [HF] | Evaluador barato; búsqueda web en "incorrecta" | ✔ el evaluador es un `choice` de 3. ✎ el "descomponer y recomponer" (resumir) no. | `choice` {suficiente, falta un eslabón, nada útil} sobre el cuaderno; "falta" → otra ronda, "nada" → abstener. |
| **EfficientRAG** (2024) | Itera **sin LLM**: un etiquetador DeBERTa-v3-large (304M) marca tokens útiles y un filtro arma la siguiente consulta. | Recall HotpotQA 81.84% con 6.41 trozos; 2Wiki 84.08% con 3.69; **MuSiQue "menos satisfactorio"** [HF] | Entrenamiento de 2 modelos pequeños; 0 LLM por iteración | ✔ marcar lo útil = nuestro "escoger frase". ✎ arma la consulta con tokens (extractivo, no generativo). | Prueba de que el bucle extractivo funciona. Idea portable: consulta = **tokens de la frase escogida** (no toda la frase) para BM25. |
| **GeAR** (2025) | Expansión de grafo sobre BM25 + agente multi-paso con haz de tripletas. | > 10% de mejora en MuSiQue, menos tokens e iteraciones [web, ACL Findings 2025] | LLM extrae tripletas por consulta | ✎ tripletas. ✔ escoger tripletas/rutas del haz. | Haz diverso sobre frases del cuaderno (propuesta 4). |

### B2. Grafos y memoria asociativa

| Arquitectura | Idea central | Ganancia reportada | Costo | Qué hace JEV / qué necesita generador | Boceto en el explorador |
|---|---|---|---|---|---|
| **HippoRAG** (2024) | OpenIE → grafo de entidades → PPR desde las entidades de la pregunta. | MuSiQue R@5 ColBERTv2 49.2 → 51.9 [plan] | LLM para indexar todo | ✎ gen-ingesta (tripletas). ✔ nada en consulta. | Ya probado como opción 3: empató con vectores (§19, §21). |
| **HippoRAG 2** (2025) | Nodos pasaje + nodos frase; empareja la pregunta con **tripletas**; filtro LLM de "reconocimiento" antes del PPR. | MuSiQue R@5 NV-Embed-v2 69.7 → 74.7 [plan; BridgeRAG lo reporta 0.747 HF]; F1 medio 59.8 vs 57.0 [HF]. Ablación: el filtro LLM aporta poco (+0.7) frente a tripleta-pregunta (+12.5) [web, no verificado] | LLM 70B indexa; LLM filtra por consulta | ✎ gen-ingesta (tripletas). ✔ el filtro de reconocimiento es un `noul` por tripleta. | Si su filtro aporta poco, la lección es **emparejar contra hechos completos** (lo que hace el cuaderno), no contra entidades. |
| **CatRAG** (2026) | Sobre HippoRAG 2: anclaje simbólico, peso de conexión dinámico según la pregunta (juez LLM), realce de pasajes con hechos clave. | Cadena completa (FCR) 34.6% vs 30.5% de HippoRAG 2; F1 MuSiQue 45.0; R@5 "ganancias modestas" [HF] | LLM juzga conexiones por consulta | ✔ juzgar una conexión es un `noul`. Ya lo probamos (opción 5): navegar no superó a juzgar la lista (§19). | Usar sus pesos dinámicos **solo como canal de candidatos** desde las fuentes del cuaderno (propuesta 3). |
| **LinearRAG** (2025, ICLR 2026) | Grafo sin relaciones (entidad–frase–pasaje) con NER ligero; activa entidades por similitud de frases y aplica PPR. | Indexar 10M tokens: 3,084 s y **0 tokens LLM** vs HippoRAG 13,815 s y 28.04M tokens; HotpotQA acierto 70.2 vs 62.9 de HippoRAG 2 [HF] | Sin LLM al indexar | ✔ nada que generar; JEV podría escoger entidades frontera. | Índice entidad → frases/pasajes (NER barato) como canal extra de la ronda ≥ 2 (propuesta 3). |
| **PropRAG** (2025, EMNLP) | Proposiciones autocontenidas en lugar de tripletas; **haz sin LLM** sobre caminos de proposiciones + PPR. | R@5 MuSiQue **78.3**, 2Wiki 94.1, HotpotQA 97.4; F1 +2.0 sobre HippoRAG 2 [HF] | Llama-3.3-70B en ingesta [GH]; 0 LLM en consulta | ✎ gen-ingesta (proposiciones). ✔ JEV puede puntuar los caminos del haz (mejor que coseno). | Proposiciones como "frases" que JEV escoge (propuesta 5) + haz de 2 caminos (propuesta 4). |
| **BridgeRAG** (2026) | Segundo salto condicionado al puente: s(q, b, c). Expansión SVO (LLM), búsqueda por 2 entidades del puente, juez tripartito. | R@5 MuSiQue **0.8146** vs PropRAG 0.783 vs HippoRAG 2 0.747; 2Wiki 0.9527; HotpotQA 0.9875. Condicionar al puente: +2.55 pp en preguntas puente-comparación, ≈0 en cadena simple [HF] | LLM 70B en consulta (SVO + juez) | ✔ el juez tripartito = nuestro `next_needed` con cuaderno. ✎ las consultas SVO. ✔ "dos entidades del puente" se obtienen **sin generar** (título de la fuente + entidad nombrada en la frase). | Búsquedas separadas por cada hecho/entidad del puente, fusionadas con RRF (propuesta 1). |
| **HopRAG** (2025) | Grafo de pasajes unidos por **pseudo-preguntas** generadas al indexar; recorre "recuperar–razonar–podar". | +76.78% métrica de respuesta y +65.07% F1 de recuperación vs RAG convencional [web] | LLM en ingesta (pseudo-preguntas) y en consulta | ✎ gen-ingesta. ✔ podar = `noul`. | Pseudo-preguntas por pasaje embebidas como vectores extra (propuesta 5). |
| **GraphRAG** (Microsoft, 2024) | Grafo de entidades + comunidades (Leiden) + resúmenes jerárquicos; map-reduce. | Exhaustividad: 72–83% de victorias vs RAG vectorial; raíz usa 97% menos tokens de contexto [HF] (juez LLM, preguntas globales) | Muy caro al indexar; consulta global cara | ✎ gen-ingesta (resúmenes). ✔ escoger comunidad. | Útil solo para preguntas globales ("¿qué dicen los protocolos sobre X?"); no ayuda a cadenas de MuSiQue. Fase posterior. |
| **LightRAG** (2024) | Grafo de entidades + recuperación de dos niveles (local/global); actualización incremental. | Recuperación < 100 tokens y 1 llamada vs GraphRAG 610,000 tokens; victorias por juez LLM [HF] | LLM en ingesta | ✎ gen-ingesta. ✔ elegir nivel (local/global) = `choice`. | Igual que GraphRAG: para preguntas temáticas. |
| **Think-on-Graph 2.0** (2025) | Alterna recuperación en grafo y en documentos (acoplamiento fuerte). | Sin cifra verificada [web] | LLM en cada paso | ✎/✔ mixto. | Equivalente al canal de entidades + cuaderno (propuesta 3). |
| **Tripletas + enlace de entidades** (KG clásico) | Normalizar menciones a entidades únicas; recorrer relaciones tipadas. | Depende del dominio; ninguna cifra comparable | Enlace de entidades caro y frágil sin catálogo | ✔ JEV decide "¿esta mención es la misma entidad que esta otra?" (`noul`) — buen uso. ✎ extraer tripletas. | En empresas/leyes, enlazar referencias explícitas ("art. 12", "procedimiento P-07") con reglas; JEV desambigua dudas. |
| **AAR** (2026) | MLP de 4.2M aprende asociaciones pasaje–pasaje del corpus y reordena. | HotpotQA R@5 0.831 → 0.916; MuSiQue +10.1 (transductivo); **inductivo sin mejora** [web] | Entrenar en co-ocurrencias del propio corpus | ✔ JEV puede **etiquetar** asociaciones (pares que co-aparecen en cuadernos). | Destilar trazas del bucle en un asociador barato; solo si hay mucho tráfico en un mismo corpus. |

### B3. Unidades, índices y árbol

| Arquitectura | Idea central | Ganancia reportada | Costo | Qué hace JEV / qué necesita generador | Boceto en el explorador |
|---|---|---|---|---|---|
| **RAPTOR** (2024) | Árbol por agrupamiento recursivo + resúmenes; búsqueda en el "árbol colapsado". | QuALITY con GPT-4 82.6% vs 62.3% del estado del arte previo; NarrativeQA +7.3 ROUGE-L sobre BM25 [HF] | Resúmenes LLM en ingesta | ✎ gen-ingesta. ✔ escoger rama. | Para documentos largos (leyes, manuales): nodos resumen como pasajes extra. |
| **PageIndex** (2025) | Sin vectores: árbol de contenidos del documento y un LLM navega por razonamiento. | FinanceBench 98.7% (afirmación del proveedor) [GH]; índice ≈ US$0.001 por página [GH] | LLM en ingesta + LLM navega en consulta | ✎ ingesta (si el documento no trae índice). ✔ **navegar es un `choice` entre títulos de secciones**: encaje natural para JEV. | Acción "abrir sección / seguir leyendo" cuando el pasaje es parte de un documento estructurado (KoBLEX: 742 artículos > 6,000 caracteres, §29). |
| **LATTICE** (2025) | Árbol semántico + LLM navega; calibra puntajes locales ruidosos en una relevancia de camino. | BRIGHT: hasta +9% Recall@100 y +5% nDCG@10 sobre el mejor cero-disparo [web] | Resúmenes en ingesta; LLM por nivel | ✔ JEV ya está calibrado: su probabilidad por nivel se multiplica a lo largo del camino. | Igual que PageIndex, con puntaje de camino = producto de P(JEV). |
| **LongRAG** (2024) | Unidades largas (4K tokens) en lugar de párrafos; lector de contexto largo. | NQ: recall@1 de respuesta 52% → 72%; corpus 22M → 600K unidades [HF] | Lector LLM de contexto largo | ✔ JEV lee 6,000 caracteres por pasaje; unidades más largas suben tokens. | Agrupar fragmentos del mismo documento para búsqueda, pero que JEV lea por secciones. |
| **Late chunking** (2024) | Embeber el documento entero y promediar tokens por trozo. | +1.5 a +1.9 pts absolutos (≈ +2.7–3.6% relativo) en promedio [HF] | Modelo de embeddings de contexto largo propio | No toca JEV. | Requiere un embedder abierto (Gemini no expone embeddings por token). Solo si se cambia de embedder. |
| **Contextual Retrieval** (Anthropic, 2024) | Un LLM barato escribe 50–100 tokens de contexto por trozo antes de embeber y de BM25. | Fallo top-20: 5.7% → 3.7% (−35%) solo embeddings; → 2.9% (−49%) + BM25; → 1.9% (−67%) + reordenador; ≈ US$1.02 por millón de tokens de documento [blog] | LLM pequeño en ingesta, una vez | ✎ gen-ingesta. JEV se beneficia: frases con contexto. | Prefijo de contexto por pasaje en el índice (propuesta 5). En MuSiQue un párrafo = un documento, así que el aporte ahí es casi solo el título. |
| **ColBERTv2** (2021) | Interacción tardía: un vector por token, MaxSim. | MS MARCO MRR@10 39.7; gana 22 de 28 pruebas fuera de dominio [HF] | Índice 6–10× más chico que ColBERT pero mayor que un vector; servidor propio | No toca JEV. | Canal de búsqueda adicional; costo de infraestructura alto para un sistema en Go. Baja prioridad. |
| **Baleen** (2021) | Recuperación "condensada" por salto: guarda solo las frases clave de cada salto. | Estado del arte en HoVer (sin cifra verificada aquí) [web] | Retriever entrenado (FLIPR) | ✔ condensar = escoger frases: **es nuestro cuaderno**. | Ya implementado; valida el diseño. |

### B4. Reordenadores multi-salto y descomposición

| Arquitectura | Idea central | Ganancia reportada | Costo | Qué hace JEV / qué necesita generador | Boceto en el explorador |
|---|---|---|---|---|---|
| **Beam Retrieval** (2023/24) | Codificador + 2 cabezas entrenados de punta a punta; mantiene varias hipótesis de cadena por salto. | ~50% de mejora sobre bases en MuSiQue-Ans (ajuste con distractores) [web] | Entrenamiento supervisado | ✔ el haz es control (código); JEV puntúa cada hipótesis. | Haz de 2 cuadernos cuando hay duda (propuesta 4). |
| **GRITHopper** (2025) | Recuperador denso 7B multi-salto sin descomponer, entrenado con objetivo generativo + embeddings. | MuSiQue Hits@1 por salto 94.25 / 76.13 / 55.45 / 32.10 [HF] | 8×A100 para entrenar; 7B por consulta | No toca JEV. | Muestra cómo cae cada salto: el 3.º y 4.º son el problema, como en nuestro 0.46/0.50 (§25). |
| **DualView** (2026) | Reordenador en cascada: puntaje local (pregunta–doc) + global (relaciones entre docs) con compuerta. | MuSiQue Top-4 Recall 99.4%, Full Hit 97.8%, 4 ms; BGE-Large 92.0% [web; ajuste de distractores, no recuperación abierta] | Entrenamiento | ✔ JEV puede generar etiquetas para entrenar uno así. | Destilación (backlog). |
| **DPS** (2025) | Selección autoregresiva del conjunto mínimo de pasajes, condicionada a los ya elegidos. | MuSiQue F1 28.85 (+3.87); "+30.06% F1 sobre Qwen3-reranker" [HF] | Ajuste fino de un LLM 7B | ✔ "¿este pasaje agrega algo a los ya elegidos?" = `next_needed` con cuaderno. | Ya es el comportamiento del bucle. |
| **Utilidad contextual** (2025) | Modelo pequeño predice la utilidad de un pasaje *dado los otros*; etiquetas desde trazas de un modelo razonador. | "Mejor reordenamiento que por relevancia"; sin cifra extraída [web] | Entrenamiento con trazas | ✔ JEV con cuaderno ya mide utilidad contextual. | Valida el diseño; útil para destilar. |
| **Descomposición de preguntas + reordenar** (2025) | LLM parte la pregunta en sub-preguntas; busca cada una; reordena la unión. | MultiHop-RAG: +4.4 Hits@4 por descomponer, +7.6 por reordenar; juntos 87.2% Hits@10 [web] | Una generación por pregunta | ✎ gen-consulta. ✔ JEV podría *escoger* entre descomposiciones plantilla, pero sin generador no hay sub-preguntas. | No portable a consulta sin generador. Sustituto: hechos como sub-preguntas resueltas (propuesta 1). |

### B5. Memoria de agente y suficiencia de evidencia

| Arquitectura | Idea central | Ganancia reportada | Costo | Qué hace JEV / qué necesita generador | Boceto en el explorador |
|---|---|---|---|---|---|
| **MemGPT / Letta** (2023) | Memoria por niveles tipo SO (contexto principal, archivo, recuerdos) gestionada por llamadas a funciones. | Deep Memory Retrieval 93.4% vs 35.3% de base (GPT-4 Turbo) [HF] | El LLM gestiona la memoria | ✔ "¿qué hecho sale del cuaderno?" y "¿esto se guarda?" son decisiones. ✎ resúmenes recursivos. | El cuaderno ya es la "memoria de trabajo" con tope 8; hoy se poda por confianza. Mejor: JEV decide qué hecho dejó de ser necesario. |
| **Zep / Graphiti** (2025) | Grafo temporal de hechos con validez (válido desde / invalidado en). | DMR 94.8% vs 93.4% de MemGPT; LongMemEval hasta +18.5% y −90% de latencia [web] | LLM extrae hechos al ingerir | ✎ gen-ingesta. ✔ "¿este hecho contradice/reemplaza a este otro?" es un `choice`. | Para procedimientos de empresa con versiones: marcar vigencia; JEV detecta conflicto entre hechos del cuaderno. |
| **Mem0** (2025) | Extraer, consolidar y recuperar recuerdos compactos. | +26% relativo en juez LLM vs memoria de OpenAI; latencia p95 1.44 s vs 17.1 s de contexto completo [web] | LLM en escritura | ✎ gen-ingesta. ✔ decidir ADD/UPDATE/DELETE es un `choice`. | Relevante para memoria conversacional, no para recuperación documental. |
| **Sufficient Context** (Google, ICLR 2025) | Distinguir "el modelo falló" de "el contexto no bastaba"; abstención guiada por señal de suficiencia + confianza. | Respuestas correctas entre las respondidas +2–10% (Gemini, GPT, Gemma) [web, resumen ICLR]; con contexto insuficiente Claude 3.5 Sonnet alucina 36.5% [HF] | Autoevaluador LLM | ✔ suficiencia = `noul` calibrado (ya: `answer_stated`, `missing_link`). | Combinar señales con regresión (propuesta 2). |
| **SURE-RAG** (2026) | Suficiencia como propiedad del **conjunto**: verificador por par (DeBERTa) → rasgos de cobertura, fuerza, incertidumbre, recuperación → regresión logística → {apoyado, refutado, insuficiente}. | Macro-F1 0.9075 (calibrado) vs 0.6516 promedio simple; riesgo a 30% de cobertura 0.164 vs 0.259 (HotpotQA-RAG) [HF] | Modelo pequeño + regresión | ✔ **calza exacto**: JEV da los puntajes por par y por conjunto; la regresión es código (`probe.FitLogistic` ya existe). | Propuesta 2. |
| **RefusalBench** (2025/26) | Banco generativo de contextos perturbados para medir rechazo selectivo. | Exactitud de rechazo < 50% en multi-documento para modelos de frontera [web] | — | ✔ JEV podría evaluarse aquí como juez de suficiencia. | Fase 4: conjunto de prueba de abstención fuera de MuSiQue. |

---

## (c) Top 5 propuestas (ordenadas por probabilidad de ayudar ÷ costo)

**Regla común.** H9 (KoBLEX, §29) está pre-registrada con la configuración de §24: nada de esto se aplica antes de que H9 corra. Cada propuesta: ajuste en dev de MuSiQue (34 + 48 preguntas, §20) o en las trazas ya guardadas; congelar; pre-registro fechado; una corrida final. Costo JEV de referencia: el bucle cuesta ≈ US$0.0018 por pregunta (§28), es decir ≈ US$0.06 en dev con respuesta (34) y ≈ US$0.27 en la memoria A (150). Con ~US$0.30 de crédito **solo cabe una corrida final completa**: las pruebas de mecanismo deben ser sin JEV o con la caché (`data/probe/cache.jsonl`; la clave es modelo + estado + preguntas, así que un estado repetido no se paga).

### 1. Consultas por hecho, con el título de su fuente (multi-consulta tipo BridgeRAG / Self-Ask sin generador)
- **Qué cambia.** Hoy, en la ronda ≥ 2, `factText` usa solo `f.Text` (sin `f.Source`) y el vector se calcula sobre la pregunta + **todos** los hechos juntos. Una frase como "He was born in 1872" pierde la entidad, y 8 hechos concatenados diluyen el puente nuevo. Cambio: para cada hecho nuevo, (a) un vector de "pregunta + `Source: Text`" y (b) BM25 de "`Source` + `Text`"; más el vector concatenado actual; fusionar todo con RRF. Es la versión sin generador de "buscar por cada entidad del puente" (BridgeRAG) y de "una sub-pregunta por paso" (Self-Ask, IRCoT).
- **Qué hace JEV.** Nada nuevo: sigue calificando `PerRound` = 10 pasajes; solo cambia qué pasajes llegan.
- **Efecto esperado.** Más alcance de pasos intermedios en la ronda 2–3. En P1, la frontera bien formada subía alcance@30 de 0.90 a 0.94 (§23); esperaría +0.02 a +0.05 en cadena@10, concentrado en 3–4 saltos. No verificado: es una predicción.
- **Riesgo.** Más ruido de BM25 con títulos genéricos (en leyes, "Ley X" se repite en miles de artículos); más llamadas de embeddings (baratas).
- **Experimento barato.** (i) Sin JEV: tomar los cuadernos reales de `results/20261003-052414-musique-train-explore-all-traces.json` y de dev; para cada pregunta medir alcance@10/@30 de los pasajes oro aún no vistos con la consulta actual vs. las variantes (a), (b), (a+b). Solo cuesta embeddings de hechos (pocos miles de tokens). (ii) Si gana ≥ +0.03 en alcance@10 de intermedios: bucle en dev (≈ US$0.06, parte en caché). (iii) Final: memoria A, cadena@10 pareada vs. bucle actual (≈ US$0.27).

### 2. Abstención por regresión sobre señales que ya existen (SURE-RAG, Sufficient Context, CRAG)
- **Qué cambia.** En lugar de la media de dos señales con umbral fijo (§26–27), una regresión logística con rasgos de conjunto: `answer_stated`, `missing_link`, filtro anterior, máximo `answers_query`, cobertura con 5 pasajes, nº de hechos, nº de rondas que agregaron hechos, si la última ronda agregó algo, dispersión de confianzas, nº de fuentes distintas. `probe.FitLogistic` ya existe.
- **Qué hace JEV.** Nada nuevo: los rasgos salen de llamadas ya hechas.
- **Efecto esperado.** AUROC 0.849 → 0.86–0.88 (las combinaciones exploratorias ya dieron 0.860–0.861 en la memoria B, §27); exactitud balanceada ≥ 0.80 plausible.
- **Riesgo.** Sobreajuste con pocas preguntas (82 en dev); la memoria B ya se miró, así que solo sirve para validación cruzada exploratoria.
- **Experimento barato.** US$0. Ajustar en dev (`results/*-musique-abstain-dev-rows.json`), validar con validación cruzada anidada por pregunta en B (`results/20261003-070134-musique-trainb-abstain-all-rows.json`), reportando AUROC con IC. La confirmación pre-registrada necesita una memoria nueva (sal "c"), que cuesta una corrida del bucle (≈ US$0.3 con 150 + 150): dejarla para cuando haya crédito, o bien hacer que coincida con la corrida final de la propuesta 1 (la misma corrida da cadena@10 y abstención).

### 3. Canal de entidades guiado por el cuaderno (LinearRAG / CatRAG / ToG-2, sin LLM)
- **Qué cambia.** Índice barato entidad → pasajes (títulos y nombres propios por reglas o NER; en KoBLEX, referencias "artículo N de la ley X"). En la ronda ≥ 2, un tercer canal en la fusión RRF: pasajes cuyo título aparece en un hecho nuevo, y pasajes que mencionan el título de la fuente del hecho. Es el grafo usado como **proveedor de candidatos condicionado a la pregunta** (lección de CatRAG), no como ordenador estático (el PPR estático empató con vectores en §19 y §21).
- **Qué hace JEV.** Califica como siempre; el grafo no decide.
- **Efecto esperado.** Ayuda donde el puente es un nombre propio que el embedding no acerca (el caso Coolidge de §27). Esperaría +0.01 a +0.04 en cadena@10; no verificado.
- **Riesgo.** Nodos "hub" (países, años) inundan el canal: limitar a entidades con frecuencia de documento baja y a 5 pasajes por entidad.
- **Experimento barato.** US$0 en JEV: la misma prueba de alcance offline de la propuesta 1, con el canal agregado; el grafo de menciones de título de MuSiQue (§20) ya existe en `graphmodel`. Si se combina con la propuesta 1, medir la contribución de cada canal por separado.

### 4. Parada adaptativa + rondas extra para las difíciles (Adaptive-RAG, CRAG, FLARE, Beam Retrieval)
- **Qué cambia.** Después de cada ronda, una llamada JEV de cobertura (`answer_stated`, `missing_link`) sobre el cuaderno: si la respuesta está dicha, parar; si no, seguir hasta 5 rondas. Opcional: cuando los dos mejores hechos candidatos vienen de pasajes distintos y sus confianzas están a < 0.15, mantener **dos cuadernos** (haz de 2) por una ronda y quedarse con el de mayor cobertura. El plan (§22.2) ya prevé "haz de 2 ramas ante duda", pero el código no lo implementa.
- **Qué hace JEV.** Decisiones de parar/seguir (`noul`) y elegir rama (`choice` o comparación de coberturas).
- **Efecto esperado.** Mismo gasto medio: 75% de las preguntas son de 2 saltos (§25) y pararían antes; ese ahorro paga rondas 4–5 para las de 3–4 saltos (cadena@10 hoy 0.46 / 0.50). Esperaría +0.05 a +0.10 en 3–4 saltos y ~0 en 2 saltos.
- **Riesgo.** Parar demasiado pronto si `answer_stated` acepta un hecho de otra entidad (tasa de falsos positivos desconocida); el haz duplica el costo en las preguntas donde se activa.
- **Experimento barato.** (i) Solo cobertura por ronda sobre las trazas guardadas: ≈ 150 × 3 llamadas cortas ≈ US$0.02; simular offline "parar en la ronda r" y ver cuánta cadena@10 se pierde y cuánto se ahorra. (ii) Rondas 4–5 solo para las 38 preguntas de 3–4 saltos de la memoria A (exploratorio, ≈ US$0.05; los estados de las rondas 1–3 están en caché).

### 5. Enriquecimiento en la ingesta con un LLM pequeño (Contextual Retrieval, PropRAG, HopRAG)
- **Qué cambia.** Una vez por pasaje, un LLM barato escribe: (a) proposiciones autocontenidas (pronombres resueltos, "Él nació en 1872" → "Calvin Coolidge nació en 1872"), que pasan a ser las "frases" que JEV escoge; (b) 2–3 pseudo-preguntas que el pasaje responde, embebidas como vectores extra que apuntan al pasaje; (c) en documentos largos, un prefijo de contexto (sección, ley, procedimiento). El texto citado al agente sigue siendo la frase original (la proposición guarda su frase de origen) para no perder la cita textual.
- **Qué hace JEV.** Lo mismo (escoger y calificar), sobre unidades más limpias. El generador nunca entra en la consulta.
- **Efecto esperado.** En MuSiQue, poco más allá de la propuesta 1 (un párrafo = un documento; el título ya da casi todo el contexto). En KoBLEX y en procedimientos de empresa (fragmentos de documentos largos) es donde Anthropic reporta −35% a −49% de fallos de recuperación. Ganancia de PropRAG atribuible a proposiciones: no aislada aquí.
- **Riesgo.** Alucinación del LLM de ingesta (una proposición que el texto no dice) → por eso la cita debe ser la frase original; en las preguntas sin respuesta de MuSiQue, el LLM podría "completar" el eslabón quitado con conocimiento propio (fuga): prohibir información externa al pasaje y auditar una muestra. Costo de ingesta en KoBLEX (44,261 artículos) del orden de unos pocos dólares con un modelo pequeño (estimación propia, no verificada; Anthropic reporta ≈ US$1.02 por millón de tokens de documento con caché).
- **Experimento barato.** Fase de mecanismo en P1 (§23), no en el bucle: enriquecer solo los 386 párrafos oro + los pasajes del pozo de esas preguntas; repetir T1 (alcance, sin JEV) y T3 (¿JEV escoge la proposición correcta?, ≈ 1,500 llamadas cortas ≈ US$0.02). Solo si T1/T3 mejoran, enriquecer la memoria completa.

### Fuera del top 5 (para después)
- **Navegación por árbol (PageIndex / LATTICE) con `choice` de JEV** entre secciones: encaje perfecto con JEV para leyes y manuales largos; hoy los conjuntos casi no lo exigen (1.7% de artículos KoBLEX > 6,000 caracteres). Es la solución de producto que §29 deja pendiente.
- **Destilar JEV en un reordenador condicionado al conjunto** (DualView, DPS, AAR, utilidad contextual): las trazas del bucle dan etiquetas gratis; bajaría el costo por consulta a ≈ 0, pero exige entrenamiento y mucho volumen en el mismo corpus (AAR no generaliza inductivamente).
- **Verificación de cita estilo Self-RAG IsSup** en la salida (fase 4).
- **Vigencia y conflicto de hechos estilo Zep** para procedimientos de empresa con versiones.
- Descartados para consulta: Self-Ask, IRCoT, FLARE, ReAct, CoRAG y Search-R1 en su forma original (escriben texto en cada paso); GraphRAG/LightRAG (resuelven preguntas globales, no cadenas); ColBERT y late chunking (cambian infraestructura de embeddings, ganancia moderada).

| # | Propuesta | Métrica principal | Efecto esperado (predicción) | Costo de la prueba de mecanismo | Riesgo |
|---|---|---|---|---|---|
| 1 | Consultas por hecho + título | alcance@10 intermedios → cadena@10 | +0.02–0.05 cadena@10 | ≈ US$0 JEV (solo embeddings) | Ruido de BM25 con títulos genéricos |
| 2 | Abstención por regresión | AUROC, exactitud balanceada | 0.849 → 0.86–0.88 | US$0 | Sobreajuste; falta memoria nueva para confirmar |
| 3 | Canal de entidades | alcance@10 → cadena@10 | +0.01–0.04 | US$0 | Nodos hub |
| 4 | Parada adaptativa + rondas extra/haz | cadena@10 en 3–4 saltos, llamadas/pregunta | +0.05–0.10 en 3–4 saltos con igual gasto medio | ≈ US$0.02–0.07 | Parar pronto por falso "respondido" |
| 5 | Enriquecimiento en ingesta | alcance (T1), elección de frase (T3), R@5 KoBLEX | Pequeño en MuSiQue; mayor en documentos largos | ≈ US$0.02 JEV + LLM de ingesta | Alucinación/fuga en la ingesta |

---

## (d) Fuentes

Artículos (resumen leído en huggingface.co/papers salvo indicación):
- IRCoT — https://huggingface.co/papers/2212.10509
- Self-Ask — https://huggingface.co/papers/2210.03350
- ReAct — https://huggingface.co/papers/2210.03629
- Self-RAG — https://huggingface.co/papers/2310.11511
- CRAG — https://huggingface.co/papers/2401.15884
- Adaptive-RAG — https://huggingface.co/papers/2403.14403
- FLARE — https://huggingface.co/papers/2305.06983
- HippoRAG 2 — https://huggingface.co/papers/2502.14802 ; código https://github.com/OSU-NLP-Group/HippoRAG ; ablación (secundaria) https://www.emergentmind.com/topics/hipporag
- CatRAG — https://huggingface.co/papers/2602.01965
- LinearRAG — https://huggingface.co/papers/2510.10114
- PropRAG — https://huggingface.co/papers/2504.18070 ; https://github.com/ReLink-Inc/PropRAG
- BridgeRAG — https://huggingface.co/papers/2604.03384
- EfficientRAG — https://huggingface.co/papers/2408.04259
- GeAR — https://preview.aclanthology.org/new-sigs/2025.findings-acl.624/ (vía buscador)
- HopRAG — https://arxiv.org/abs/2502.12442 (vía buscador)
- GraphRAG — https://huggingface.co/papers/2404.16130
- LightRAG — https://huggingface.co/papers/2410.05779
- Think-on-Graph 2.0 — https://iclr.cc/virtual/2025/poster/28367 (vía buscador)
- RAPTOR — https://huggingface.co/papers/2401.18059
- PageIndex — https://github.com/VectifyAI/PageIndex
- LATTICE — https://arxiv.org/abs/2510.13217 (vía buscador)
- LongRAG — https://huggingface.co/papers/2406.15319
- Late chunking — https://huggingface.co/papers/2409.04701
- Contextual Retrieval — https://www.anthropic.com/news/contextual-retrieval
- ColBERTv2 — https://huggingface.co/papers/2112.01488
- Baleen — https://neurips.cc/virtual/2021/poster/26450 (vía buscador)
- Beam Retrieval — https://aclanthology.org/2024.naacl-long.96 (vía buscador)
- GRITHopper — https://huggingface.co/papers/2503.07519
- DualView — https://arxiv.org/abs/2605.18767 (vía buscador)
- DPS (Dynamic Passage Selector) — https://huggingface.co/papers/2508.09497
- Utilidad contextual de pasajes — https://arxiv.org/abs/2512.06464 (vía buscador)
- AAR (Association Is Not Similarity) — https://arxiv.org/abs/2604.20850 (vía buscador)
- Contrastive Evidence Exploration — https://arxiv.org/abs/2609.07050 (vía buscador; usa Qwen2.5-7B para generar consultas, no se extrajeron cifras)
- Descomposición de preguntas para RAG — https://arxiv.org/abs/2507.00355 (vía buscador)
- CoRAG — https://arxiv.org/abs/2501.14342 (vía buscador)
- Search-R1 — https://arxiv.org/abs/2503.09516 (vía buscador)
- MemGPT — https://huggingface.co/papers/2310.08560
- Zep — https://huggingface.co/papers/2501.13956 (vía buscador)
- Mem0 — https://arxiv.org/abs/2504.19413 (vía buscador)
- Sufficient Context — https://huggingface.co/papers/2411.06037 ; https://github.com/hljoren/sufficientcontext
- SURE-RAG — https://huggingface.co/papers/2605.03534
- RefusalBench — https://arxiv.org/abs/2510.10390 (vía buscador)

Internas: `plan.md` §17–29; `internal/explorer/explorer.go` (`Explore`, `factText`, `scorePassages`); `internal/notebook/judge.go`; `internal/probe/metrics.go` (`FitLogistic`); resultados citados en `results/`.
