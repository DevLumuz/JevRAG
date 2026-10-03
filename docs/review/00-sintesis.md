# Síntesis: revisión de la arquitectura (3 de octubre de 2026)

Fuentes: tres revisiones independientes (`01-contenido-largo.md`, `02-puntos-ciegos.md`, `03-arquitecturas-inspiracion.md`), mi propia revisión del código y las pruebas H6–H9 (`plan.md` §24–30).

## 1. Dónde estamos, en una tabla

| Pregunta | Respuesta medida | Estado |
|---|---|---|
| ¿El cuaderno ayuda cuando la cadena tiene eslabones que no se parecen a la pregunta? | Sí: cadena completa 73% → 83% en preguntas nuevas de MuSiQue (H6) | ✅ |
| ¿Un reordenador estándar hace lo mismo que JEV? | No: lo empeora (63%) (H8) | ✅ JEV se queda |
| ¿Sabe decir "no hay evidencia"? | AUROC 0.849, meta 0.85 (H7); combinar las señales actuales no pasa de 0.85 | ⚠️ casi |
| ¿Sirve en leyes? | Gana a la búsqueda simple (+8.5) pero empata con el filtro de JEV, que es 2.8× más barato (H9) | ❌ no aporta sobre lo simple |
| ¿El agente recibe la evidencia? | El documento correcto está en el top 10 en 92% de los casos, pero el texto de la respuesta está en el cuaderno solo en 57% | ❌ hueco importante |

**Lectura de fondo:** el cuaderno no es una mejora universal. Ayuda cuando hay eslabones "escondidos" (preguntas que encadenan cosas que no se mencionan) y no aporta cuando todo lo necesario se parece a la pregunta. El producto debe **decidir cuándo explorar**: una sola pasada barata si la evidencia ya está, rondas extra solo si falta un eslabón.

## 2. Contenidos muy grandes (tu pregunta original)

**Lo que encontramos:**
- El artículo de 5.2 millones de caracteres de KoBLEX es un error de los datos: una ventana de ~7,800 caracteres repetida 1,328 veces. Los artículos largos reales miden de 6,000 a 46,000 caracteres.
- KoBLEX casi no pone a prueba documentos largos: solo 13 de 443 respuestas correctas viven en artículos de más de 6,000 caracteres.
- Había un fallo: la revisión de suficiencia enviaba artículos completos a JEV, sin cortar. Ya está corregido.

**La solución recomendada (las dos piezas juntas):**
1. **Partir al guardar, según la estructura del documento.** Ley → artículo → párrafo → fracción → inciso. Procedimiento → sección → paso. En otros documentos, los encabezados. Fragmentos de 300 a 1,500 caracteres (máximo 2,500) que nunca cruzan un encabezado. Cada fragmento lleva su ruta como título ("Ley X › Art. 47 › fracc. III"), y los incisos de una lista llevan también la frase que los introduce. No necesita LLM y no cuesta nada.
2. **Enlaces calculados en código:** padre, anterior, siguiente, referencias explícitas ("ver artículo 40") resueltas a nivel de párrafo, y una tabla de términos definidos. Al cargar, se limpia la basura repetida.
3. **Acción "seguir leyendo" en el explorador.** El código propone de 2 a 6 candidatos por fragmento leído (el siguiente, el padre, la referencia citada…). JEV decide con una pregunta sí/no sobre un adelanto corto de cada uno. Los aceptados entran a la ronda como pasajes normales. Para documentos muy largos hay un modo "índice": JEV califica hasta 12 encabezados de sección y el bucle entra solo a los elegidos.
4. **Topes:** 4 expansiones por ronda, 8 fragmentos por documento, profundidad 2, 40,000 caracteres leídos por pregunta.
5. **Citas a nivel de fragmento:** cada hecho del cuaderno guarda documento, fragmento, rango de caracteres y cómo se llegó a él.

Las preguntas JSON exactas para JEV están en `01-contenido-largo.md`, sección (c).

## 3. Otras cosas que se nos estaban pasando (priorizadas)

**P0, antes de llamarlo producto:**
1. **El agente recibe el cuaderno, no el top 10, y al cuaderno le faltan datos.** Arreglo barato: una "lectura final" de los 5 mejores pasajes con el cuaderno completo (unas 18 llamadas más), y medir lo que el agente realmente recibe: la respuesta y sus citas.
2. **Faltan las salvaguardas del plan §22.2:** comparar números y fechas en código, confianza limitada por la del hecho que lo originó, contradicciones, corroboración y guardia contra instrucciones ocultas. Además, poner primero cualquier pasaje que aportó un hecho amplifica un hecho equivocado.
3. **Tiempo y permisos:** vigencia y versión de leyes, reformas, fechas de entrada en vigor, jurisdicción, y quién puede ver qué documento. La caché de JEV hoy se comparte entre consultas, y en producción debe respetar permisos.
4. **Escala:** la búsqueda recorre toda la memoria en cada consulta. Con 1 millón de fragmentos serían ~25 GB de RAM. Hace falta un índice aproximado (HNSW o pgvector) y medir latencia.
5. **No hay evidencia en español ni en documentos de empresa.** El BM25 no normaliza acentos y parte números, fechas y códigos.

**P1:**
6. **JEV no es del todo determinista.** 87 de 134 respuestas repetidas difieren (hasta 0.09) y 5 cruzan el umbral de 0.5. Hay que medir la varianza con repeticiones. El envío duplicado ya se arregló.
7. **Reutilizamos conjuntos de preguntas** (H6 y H8 usan las mismas 150; B comparte 12 con A) y hay pocos casos de 3–4 saltos (26 y 12).
8. **La abstención solo se probó con un tipo de "sin respuesta"** (falta un párrafo, mitad y mitad). Por saltos: 0.88 / 0.80 / 0.67. Combinar las señales actuales no pasa de 0.85 (lo probé: 0.849). Para mejorarla hace falta información nueva, por ejemplo qué requisito de la pregunta cubre cada hecho.
9. **El divisor de frases rompe texto legal y de PDF:** saltos de línea, "1.", "(a)", ";" y etiquetas como `<Amended…>`. Ya agregué las abreviaturas en español y los signos ¿ ¡ «.
10. **Una frase aislada pierde sus excepciones** ("salvo que…") y sus definiciones. Las citas usan el título del documento, no el fragmento. No hay deduplicación.

Fallos de código ya corregidos en esta revisión:
- llamadas duplicadas a JEV;
- §29 del plan repetida;
- falta de mapeo canónico en la abstención;
- evidencia sin cortar en la revisión de suficiencia.

Pendientes:
- una respuesta faltante de JEV se lee como 0;
- el cliente HTTP de JEV no tiene timeout;
- el recorte del cuaderno usa el texto del hecho como llave.

## 4. Arquitecturas de inspiración (lo más útil de 41 revisadas)

- **BridgeRAG (2026)** es la más parecida a la nuestra: califica cada candidato contra la pregunta + el puente ya encontrado. Reporta R@5 0.815 en MuSiQue, por encima de HippoRAG 2 (0.747). Su paso clave es una decisión sí/no, justo lo que JEV hace con el cuaderno. Confirma la dirección.
- **PropRAG y EfficientRAG:** el bucle puede funcionar sin un generador al consultar. El LLM se usa una vez al guardar (proposiciones, contexto) o se reemplaza con clasificadores pequeños. Encaja con JEV.
- **Sufficient Context / SURE-RAG:** la abstención se decide sobre el conjunto de evidencia con varias señales. Es nuestro camino para la Fase 3.
- La mayoría de los métodos iterativos (IRCoT, Self-Ask, ReAct, FLARE…) necesitan un generador en cada paso. JEV puede tomar sus **decisiones** (seguir, parar, ¿alcanza?), pero no escribir sus consultas.

## 5. Plan recomendado (en orden)

| # | Qué | Por qué | Costo JEV |
|---|---|---|---|
| 1 | **Partición por estructura + enlaces + "seguir leyendo"** | Resuelve los documentos grandes y da citas precisas | Partir: $0. Probar: ~US$1 (MuSiQue con párrafos enterrados en documentos largos) |
| 2 | **Lectura final + medir lo que recibe el agente** (respuesta y citas) | Cierra el hueco 92% → 57% | ~US$0.10–0.30 |
| 3 | **Exploración adaptativa:** una pasada; rondas extra solo si JEV dice que falta un eslabón | Toma lo mejor de H6 (MuSiQue) y H9 (leyes) a menor costo | ~US$0.3 |
| 4 | **Búsqueda por cada hecho nuevo con su título de fuente** | La búsqueda de la ronda 2 hoy pierde la entidad cuando el hecho usa pronombres | $0 para la primera prueba |
| 5 | **Prueba con tus documentos en español** (30–50 preguntas tuyas, algunas sin respuesta) | Es la prueba que de verdad dice si sirve como producto | ~US$0.3 |

Con el saldo actual de JEV (~US$0.13) solo alcanzan las pruebas de costo cero (4, y el diseño y código de 1). Para el resto hace falta una recarga pequeña: US$3–5 cubren todo el plan.
