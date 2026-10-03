# Revisión 01 — Contenido largo: partir documentos y "seguir leyendo"

*Revisión de arquitectura, 3 de octubre de 2026. Solo lectura del código; no se llamó a Gemini ni a JEV. Las cifras de KoBLEX salen de `.cache/koblex-statutes.rows.json` y `.cache/koblex-qa.rows.json` (análisis local, gratis).*

---

## (a) Resumen en 10 líneas

1. Hoy un nodo es un artículo o párrafo entero: el embedding corta a 8,500 caracteres, JEV ve 6,000 y como máximo 25 frases. Lo que viene después **no existe** para el sistema.
2. Hallazgo: el artículo de 5.2M de caracteres de KoBLEX es **basura de extracción** (la misma ventana de unos 7,800 caracteres repetida 1,328 veces, corrida 6 caracteres cada vez). Los 7 artículos de más de 100k son así. Los largos de verdad miden entre 6k y 46k.
3. En KoBLEX solo 13 de los 443 contextos oro caen en artículos largos, y solo unos 2 empiezan después del carácter 8,500. **KoBLEX casi no mide este problema.** Hacen falta datos con documentos largos: ConditionalQA (procedimientos), QASPER (artículos con secciones), LegalBench-RAG (contratos, oro por rango de caracteres) y un "documento largo sintético" armado con MuSiQue.
4. Propuesta: **partir al ingerir según la estructura** (ley → artículo → párrafo → numeral → inciso; manual → sección → paso; títulos de Markdown/HTML/PDF), en hojas de unos 300–1,500 caracteres. Cada hoja lleva su **ruta de títulos** y enlaces a su padre, a la hoja anterior y a la siguiente, y a las referencias que cita. Todo es determinista: sin LLM y sin costo.
5. Con eso, cada pedazo cabe completo en lo que JEV lee: se acaba el problema de "solo ve el inicio".
6. En el explorador, una acción nueva, **"expandir"**: por cada hoja leída, el código arma de 2 a 6 candidatos (siguiente, anterior, encabezado o regla general del padre, referencia explícita, definición). JEV **solo califica** cada candidato con un `noul` sobre una vista previa corta, y los mejores entran a la ronda como pasajes más. Para documentos muy largos hay un modo **"índice"**: JEV elige secciones a partir de los títulos (estilo PageIndex, pero clasificando, no razonando).
7. El cuaderno guarda la procedencia a nivel de hoja (`doc`, ruta, rango de caracteres, por qué camino llegó). La evaluación se hace a **dos niveles**: documento (como hoy) y pedazo/rango (oro mapeado a hojas), con presupuesto en caracteres leídos.
8. El enriquecimiento con un LLM pequeño al ingerir ("contextual retrieval" de Anthropic) es opcional y se prueba **después**. La ruta de títulos gratis da buena parte del efecto.
9. Errores ya presentes en el código: `abstain.go` y `judge.EvidenceSufficiency` mandan `n.Content` **sin cortar** a JEV (en KoBLEX un artículo puede volver el estado enorme); el separador de frases no corta en `;` ni en "1." o "(a)" de las leyes, y deja pasar etiquetas `<Amended…>`.
10. Plan de experimentos: E0 auditoría (gratis) → E1 MuSiQue con documentos largos sintéticos (~US$1) → E2 banco de la pregunta "expandir" (~US$0.10) → E3 ConditionalQA (~US$1.5) → E4 KoBLEX partido, sin regresión (~US$2.5) → E5 QASPER / LegalBench-RAG-mini (~US$3–4). Total por debajo de US$10.

---

## (b) Propuesta de arquitectura

### b.1 Vista general

```
                         INGESTA (una vez, sin LLM por defecto)
 documento crudo ─► normalizar ─► detectar basura ─► parser de estructura ─► árbol
 (ley, manual,      (espacios,     (ventanas repetidas,   (perfil: legal /         │
  md, html, pdf)     etiquetas)     duplicados casi        markdown / html /       │
                                    iguales)               pdf-títulos / plano)    ▼
                                                   ┌─────────── Árbol del documento ───────────┐
                                                   │ Doc ─► Sección/Capítulo ─► Unidad ─► Hoja │
                                                   │  (ley)   (título/cap.)    (artículo) (párrafo,│
                                                   │                                   numeral)│
                                                   └───────────────────────────────────────────┘
                                                                     │
          ┌──────────────────────────────┬───────────────────────────┼──────────────────────────┐
          ▼                              ▼                           ▼                          ▼
   HOJAS (índice principal)     NODOS SECCIÓN (índice 2)     ENLACES (graphmodel.Edge)   TABLA DE DEFINICIONES
   300–1,500 caracteres         ruta + título + primeras     child_of, next, prev,       término → hoja que lo
   texto embebido =             líneas + títulos de los      references (a hoja o        define (alcance: ley /
   "Doc › Sec › Art (n)"        hijos ("tabla de             unidad), defines,           documento)
   + encabezado del padre       contenido extractiva")       same_unit
   + texto de la hoja
          │                              │
          └────────────── vector (Gemini) + BM25 ─────────────┘

                         CONSULTA (explorador con cuaderno, por rondas)
 pregunta ─► ronda r: búsqueda híbrida sobre HOJAS (y secciones) ─► nuevos pasajes
                │
                ▼
     JEV califica cada pasaje (next_needed, answers_query) — reordena, no descarta
                │
                ▼
     JEV elige frases clave de los mejores ─► CUADERNO {frase, doc, ruta, rango, vía, ronda}
                │
                ▼
     EXPANDIR (nuevo): por cada hoja leída con puntaje ≥ τ, el código arma candidatos
        ├─ siguiente / anterior (misma unidad)
        ├─ encabezado de la unidad ("Cualquiera de las siguientes personas…")
        ├─ referencia explícita resuelta ("art. 34 (2) 1")
        ├─ definición de un término usado en la frase
        └─ (doc muy largo) secciones del índice
        JEV: un noul por candidato sobre {pregunta, cuaderno, frase actual, vista previa}
        ─► los mejores (≤ E por ronda) entran como pasajes de la MISMA ronda
                │
                ▼
     cobertura del cuaderno ─► parar / otra ronda / abstenerse
                │
                ▼
 salida: hojas ordenadas + cuaderno con citas exactas (doc › ruta › caracteres a–b)
```

### b.2 Ingesta y partición

**Paso 0 — normalizar y detectar basura (gratis, obligatorio).**
- Quitar etiquetas editoriales del texto que se juzga, no solo del que se embebe. Hoy `legal.CleanForEmbedding` limpia solo el texto embebido y JEV ve `<Amended on …>` y `<img id=…>`. Guardar el original para citar.
- Detectar texto repetido: dividir por `<img …>` o en tejas (shingles) de 200 caracteres. Si el texto comprimido pesa menos del 5% del original (el de 5.2M pesa 0.76%), quedarse con la ventana única más larga y marcar `properties["dedup"]="sliding-window"`. Así los 7 artículos gigantes de KoBLEX quedan en unos 8–46k.
- Duplicados casi iguales entre documentos (versiones, enmiendas): MinHash a nivel de hoja; enlazarlos con `same_text` y no indexarlos dos veces.

**Paso 1 — parser de estructura por perfil.** El árbol es genérico. Solo el reconocedor de títulos cambia según el tipo de documento:

| perfil | niveles y marcadores | fuente |
|---|---|---|
| legal (KoBLEX/Corea, inglés) | Ley → Capítulo/Sección (si viene) → Artículo `Article N (Título)` → párrafo `(1)` → numeral `1.` / `2-2.` → inciso `(a)` → ítem `(i)`; "Provided, That…" = salvedad que va con su párrafo | `index_eng` + regex sobre `content_eng` |
| legal (español/LatAm) | Ley → Libro/Título/Capítulo/Sección → Artículo → párrafo/inciso → fracción `I.`/`II.` → apartado `a)` | Akoma Ntoso (`ANhier`) como vocabulario común |
| procedimientos | Documento → Sección (h1/h2) → Subsección → Paso numerado → nota/advertencia | títulos Markdown/HTML, listas ordenadas |
| markdown/html | `#`…`######`, `<h1>`…`<h6>`, `<section>`, `<li>`, `<table>` | árbol del DOM / mdast |
| pdf | títulos detectados (tamaño de letra, numeración "3.2.1"), páginas | Docling o similar; la página como respaldo |
| plano | párrafos (línea en blanco) → ventanas de frases | — |

**Paso 2 — reglas de tamaño de las hojas.**
- Objetivo: **300–1,500 caracteres** (≈ 70–350 tokens); tope duro de **2,500**. Las pruebas publicadas coinciden: pedazos de 64–128 tokens rinden mejor para respuestas puntuales y de 512–1,024 cuando hace falta contexto amplio (Fraunhofer 2025); el divisor recursivo por fronteras naturales le gana al de tamaño fijo (Chroma; LegalBench-RAG); el *semantic chunking* no compensa su costo (2024). En leyes, el párrafo `(n)` ya tiene ese tamaño: en los 742 artículos largos de KoBLEX la mediana es 225 caracteres y el p90 839.
- **Nunca cruzar** una frontera de título. Juntar hermanos pequeños (< 300) hasta ~1,200. Un hijo grande (> 2,500) se divide en sus propios hijos (numerales, incisos); si no tiene, por frases, con **1 frase de solapamiento**.
- **Regla del encabezado (crítica en leyes y procedimientos).** Los numerales e incisos dependen de su frase de entrada ("Cualquiera de las siguientes personas pagará…:", "Para solicitar, haga lo siguiente:"). Cada hoja hija lleva el encabezado de su padre como **prefijo de contexto** (embebido y mostrado a JEV), recortado a 300 caracteres. Sin esto, "2. Una persona que pretende desviar tierras de cultivo…" no dice qué obligación tiene.
- **Tablas:** completas si caben en 2,500; si no, por grupos de filas **repitiendo la fila de encabezado**, con el título de la tabla en la ruta. Fila = registro: nunca cortar dentro de una fila.
- **Listas:** el ítem con su frase de entrada; las listas cortas, enteras.
- **Definiciones:** un artículo o sección "Definiciones" (p. ej. `CREDIT INFORMATION USE AND PROTECTION ACT / Article. 2 / Definitions`, de 18,883 caracteres) se parte en una hoja por término y se llena una **tabla de términos** (`"término" means …`, `"X" significa …`). Cada hoja que usa un término definido recibe un enlace `defines` hacia esa definición.
- **Ruta de títulos (gratis, siempre).** Texto embebido = `Nombre de la ley › Capítulo › Artículo 38 (Cargos de preservación) › (1) 2.` + encabezado del padre + texto de la hoja. Es el `contextualize()` de Docling. Para JEV, `Source` pasa a ser esa ruta, no solo la llave.

**Paso 3 — enlaces (todos como `graphmodel.Edge`, en código, sin modelo).**
- `child_of` / `parent`, `next` / `prev` (orden dentro de la misma unidad y entre unidades hermanas).
- `references`: el regex actual (`Article N`) se extiende a la forma completa "Article 121-9 (1) 1 of the Act", "paragraph (2)", "subparagraph 5 of Article 2 of X Act", "véase el artículo 40, fracción II", "see section 3.2". Se resuelve **a la hoja más específica que exista**: el artículo si no hay párrafo. Las referencias "of the Act" en un decreto de aplicación apuntan a la ley madre: resolverlas con el nombre del decreto ("ENFORCEMENT DECREE OF THE X ACT" → "X ACT"). Hoy `BuildEdges` las descarta por ser de otra ley.
- `defines` (término → hoja que lo define), `same_text` (duplicados).

**Paso 4 — granularidad múltiple.**
- **Índice principal: las hojas.** Es donde se busca.
- **Índice 2: nodos sección y unidad** con texto *extractivo*: ruta + título + primeras ~400 caracteres + títulos de los hijos. Sirve para preguntas generales ("¿qué regula el capítulo de sanciones?") y para el modo índice. JEV no escribe, así que no hay resúmenes generativos por defecto.
- **Resúmenes generativos (RAPTOR, PageIndex, ReadAgent): solo como experimento**, con un LLM pequeño al ingerir y con el texto marcado `origin="llm-summary"`. **Nunca se cita un resumen**: la cita siempre apunta a una hoja literal. RAPTOR reporta ganancias en QASPER/QuALITY con GPT-4 leyendo, y la variante "árbol colapsado" (todo en un índice) le gana a la navegación de arriba abajo. Eso apoya buscar sobre hojas + secciones en un solo índice y no obligar a descender.
- **Contexto generado ("contextual retrieval" de Anthropic):** 50–100 tokens de contexto por pedazo escritos por un LLM que ve el documento entero. Bajó las fallas de recuperación 35% (solo embeddings), 49% (+BM25) y 67% (+ reordenador), a US$1.02 por millón de tokens de documento con caché. Para nosotros es el **experimento E6**, contra la ruta de títulos gratis. En el estudio de Bologna (ECIR 2025), el contexto generado conservó mejor el sentido que el *late chunking*, a mayor costo.
- ***Late chunking*** (Jina): embeber el documento entero y promediar los tokens de cada pedazo. Requiere salidas por token, y la API de Gemini no las da. **No aplica** con el embedding actual; queda descartado salvo que se cambie de modelo.

**Paso 5 — modelo de nodo.** `graphmodel.Node` ya sirve. Se agregan propiedades: `doc` (llave del documento), `path` (ruta legible), `level` (leaf|unit|section|doc), `ordinal`, `char_start`, `char_end` (sobre el texto original normalizado), `kind` (text|table|list|definition|heading), `lead` (encabezado del padre). La llave de hoja es `doc#ruta`, p. ej. `FARMLAND ACT#38(1)2`.

### b.3 Exploración: "seguir leyendo"

**Principio:** el código propone y JEV califica. JEV no "navega" ni razona en varios pasos (MemWalker documenta que los modelos débiles no se benefician de navegar un árbol). Cada decisión es un `noul` independiente sobre un estado pequeño (< 2,500 caracteres), del mismo tipo que los que ya validamos (P1: elegir frases 0.89; reconocer el siguiente eslabón con cuaderno 0.87).

**Tres acciones nuevas:**

1. **Expandir (local).** Tras leer las frases de una hoja `h` con puntaje ≥ τ_exp (inicial 0.5), el código arma sus candidatos:
   - `next` / `prev`: hermanos inmediatos dentro de la unidad (si `h` termina en ":" o en "any of the following", `next` es obligatorio y no gasta llamada).
   - `lead`: el encabezado del padre, si `h` es un numeral o inciso y el encabezado aún no está en el cuaderno.
   - `ref:<k>`: cada referencia explícita resuelta que aparece en **las frases elegidas** (no en toda la hoja), máximo 3.
   - `def:<término>`: cada término definido que aparece en las frases elegidas, máximo 2.
   - Cada candidato se muestra con **relación en palabras** ("the next paragraph of the same article", "the article that this sentence cites as 'Article 34 (1)'"), su ruta y una vista previa de 300 caracteres. JEV lee al pie de la letra, así que la relación debe estar escrita.
   - Pregunta `expand_useful` (noul) por candidato. Los de P ≥ τ_e (0.5) pasan a la ronda **como pasajes nuevos** y se califican y leen igual que los de la búsqueda (`PassageQuestions` + frases). No se descartan: los que no pasan quedan en la cola con su puntaje.
2. **Índice (documento largo).** Si una hoja muy bien calificada (answers_query o next_needed ≥ 0.7) pertenece a un documento con más de N=20 hojas, el código muestra las **secciones hermanas** de la sección actual (≤ 12, título + 1 línea) y JEV califica cada una con `section_relevant` (noul). Se abren las 1–2 mejores, empezando por la hoja de esa sección más parecida por vector a pregunta + cuaderno. Es PageIndex hecho con un clasificador y con código.
3. **Seguir referencia en la siguiente ronda.** Las referencias que no se expandieron se agregan a la búsqueda de la ronda siguiente como BM25 de la llave canónica (gratis), además de pregunta + hechos.

**Presupuestos y reglas de parada (valores iniciales para ajustar en dev):**

| parámetro | valor inicial | por qué |
|---|---|---|
| hojas leídas por ronda (`ReadTop`) | 3 (igual que hoy) | comparabilidad con H6 |
| candidatos de expansión por hoja leída | ≤ 6 | estado pequeño, costo acotado |
| expansiones aceptadas por ronda (E) | ≤ 4 | evita que un documento acapare la ronda |
| hojas por documento por pregunta (D) | ≤ 8 | contra el "agujero negro" de un documento enorme |
| profundidad de expansión encadenada | 2 (expandir lo expandido una vez) | evita caminar todo el documento |
| caracteres leídos por pregunta | ≤ 40,000 | presupuesto principal, también para comparar con "todo el documento" |
| parada local | 2 expansiones seguidas con P < 0.3 en la misma dirección | "ya salí de la parte relevante" |
| parada global | cobertura (`answer_stated` ≥ 0.7 y `missing_link` ≤ 0.3) o presupuesto | igual que Fase 3 |
| llamadas JEV por pregunta | ~66 hoy → tope 110 | ≈ US$0.003 por pregunta |

**Cuaderno con procedencia por hoja.** `notebook.Fact` pasa de `{Text, Source}` a:

```go
type Fact struct {
    Text      string `json:"text"`
    Source    string `json:"source"`     // ruta legible: "Farmland Act › Art. 38 (1) 2."
    Doc       string `json:"doc"`        // llave del documento
    Chunk     string `json:"chunk"`      // llave de la hoja
    CharStart int    `json:"char_start"` // rango de la frase en el texto original
    CharEnd   int    `json:"char_end"`
    Via       string `json:"via"`        // search | next | prev | lead | ref | def | toc
    From      string `json:"from"`       // hoja desde la que se expandió (vacío si vino de búsqueda)
    Round     int    `json:"round"`
}
```

La vista que ve JEV (`FactView`) sigue siendo `{text, source}`. Lo demás es para citar, auditar y medir. Regla: la confianza de un hecho que llegó por expansión no supera la del hecho de su hoja de origen (§22.2, "confianza acotada por la del padre").

**Ranking y evaluación.**
- La salida se ordena por **hojas**. La métrica a nivel de documento colapsa hojas a documento (primera aparición), como hoy `Canon`: así se comparan con H6/H9 sin cambiar nada.
- **Oro a nivel de pedazo:** KoBLEX trae el texto de cada contexto oro a nivel de párrafo (mediana 325 caracteres). Se mapea por subcadena normalizada a la(s) hoja(s) que lo cubren. Las pruebas locales encontraron casi todos; los que fallan son por la marca `%MACHINE_TRANSLATED%`, que hay que quitar antes de comparar. MuSiQue: oro = párrafo = hoja. ConditionalQA/QASPER: oro = párrafo/frase de evidencia. LegalBench-RAG: oro = rango de caracteres.
- **Métricas nuevas:** (1) *recall de rango @ presupuesto*: fracción de caracteres oro cubiertos por las hojas devueltas dentro de B = 2k/5k/10k caracteres (al estilo Chroma, IoU de tokens); (2) cadena@10 a nivel de hoja y a nivel de documento; (3) *precisión de lectura*: fracción de hojas leídas o expandidas que contienen oro; (4) caracteres leídos y llamadas JEV por pregunta; (5) estratificar por **posición del oro dentro del documento** (0–6k, 6–20k, > 20k caracteres): es la prueba directa del problema.

---

## (c) Preguntas JEV exactas (JSON)

Formato igual al de `internal/notebook/judge.go` (`instructions` y `criteria` en el orden escrito, `jev.Ordered`). Instrucciones en inglés, como las ya validadas.

### c.1 Estado de expansión

```json
{
  "query": "…pregunta (+ caso)…",
  "known_facts": [{"text": "…", "source": "Farmland Act › Art. 38 (1) 1."}],
  "current": {
    "source": "Farmland Act › Article 38 (Farmland Preservation Charges) › (1) 2.",
    "sentence": "A person who intends to divert farmland … under Article 34 (2) 1;"
  },
  "candidate": {
    "relation": "the article that `current.sentence` cites as \"Article 34 (2) 1\"",
    "source": "Farmland Act › Article 34 (Permission to Divert Farmland) › (2) 1.",
    "preview": "…primeros 300 caracteres…"
  }
}
```

### c.2 `expand_useful`: ¿vale la pena leer este candidato?

```json
{
  "expand_useful": {
    "type": "noul",
    "instructions": {
      "question": "Is `candidate` likely to state a fact still needed to answer `query`?",
      "focus": "`current.sentence` was just read from a long document; `candidate` is a nearby or cited part of the same body of documents, described by `candidate.relation`, and `candidate.preview` is only its beginning. `known_facts` are facts already collected. A candidate is useful when it completes, conditions, defines or continues what `current.sentence` says in a way `query` depends on."
    },
    "criteria": {
      "true": "The candidate is the continuation, the governing condition or exception, the cited provision, or the definition of a term, that `query` needs and that `known_facts` do not already state.",
      "false": "The candidate covers another topic, repeats `known_facts` or `current.sentence`, or `current.sentence` is already complete for what `query` asks."
    }
  }
}
```

### c.3 `continues` (gratis si el texto termina en ":"; si no, una llamada): ¿la hoja quedó incompleta?

Estado: `{query, known_facts, current: {source, text}}`, con `text` = la hoja completa (≤ 2,500 caracteres).

```json
{
  "continues": {
    "type": "noul",
    "instructions": {
      "question": "Does the part of `current.text` that matters for `query` continue beyond its end?",
      "focus": "`current.text` is one piece of a longer document. Look at how it ends: a list announced but not given, a rule whose exception or condition is announced (\"except as provided in…\", \"subject to the following\"), or a step sequence cut off."
    },
    "criteria": {
      "true": "The relevant statement is cut off or announces items, conditions or steps that are not in `current.text`.",
      "false": "The relevant statement is complete in `current.text`, or `current.text` is not relevant to `query`."
    }
  }
}
```

### c.4 `section_relevant` (modo índice): una llamada por sección (≤ 12), no un `choice`

Se usa un noul por sección porque en P1/T3 el `choice` eligió "none" de más en 70 de 386 casos, y porque un noul por opción se puede cachear y paralelizar.

Estado: `{query, known_facts, document: "Ley X", section: {path, title, first_line, child_titles: [...≤8]}}`

```json
{
  "section_relevant": {
    "type": "noul",
    "instructions": {
      "question": "Is `section` the part of `document` where a fact still needed for `query` is most likely stated?",
      "focus": "Only titles and the first line are shown. Use the words of `query` and `known_facts`: the item, the action, the condition or the value asked. Do not assume content that the titles do not suggest."
    },
    "criteria": {
      "true": "The title or first line names the item, action, condition, procedure step or value that `query` or `known_facts` point to.",
      "false": "The section is about other items or actions, or only shares general vocabulary with `query`."
    }
  }
}
```

### c.5 `needs_definition`: ¿un término definido cambia la respuesta?

Estado: `{query, known_facts, sentence, term, definition_preview}`

```json
{
  "needs_definition": {
    "type": "noul",
    "instructions": {
      "question": "Does answering `query` depend on what `term` means as defined in `definition_preview`?",
      "focus": "`term` appears in `sentence`; documents often give ordinary words a narrower legal or internal meaning. Judge whether the definition decides whether the case in `query` is covered."
    },
    "criteria": {
      "true": "The definition includes, excludes or limits something `query` asks about (a person, item, amount, period or situation).",
      "false": "The term is used in its ordinary sense for `query`, or `query` does not depend on it."
    }
  }
}
```

### c.6 Cobertura con salvedades (extensión de Fase 3, opcional)

Agregar a `CoverageQuestions` (mismo estado `{query, known_facts}`):

```json
{
  "unread_exception": {
    "type": "noul",
    "instructions": {
      "question": "Do `known_facts` point to an exception, condition or proviso that has not been collected?",
      "focus": "Rules often say \"except as provided in…\", \"Provided, That…\", \"unless…\", \"subject to Article N\". Check whether such a pointer appears and its content is missing from `known_facts`."
    },
    "criteria": {
      "true": "A fact announces or cites an exception, condition or proviso whose content no fact states.",
      "false": "No fact announces such a pointer, or its content is already stated by some fact."
    }
  }
}
```

Si `unread_exception` es alto, se dispara una ronda extra de expansión `ref` antes de parar. Si el presupuesto se acaba, sale como señal de "respuesta condicional / incompleta" para el agente.

**Cuidados con las debilidades conocidas de JEV.** (1) Indirección: la relación del candidato va escrita y el cuaderno se resuelve en el estado; no se pide encadenar dos saltos. (2) Estados grandes: vista previa ≤ 300 caracteres, ≤ 8 hechos, hoja ≤ 2,500. (3) Lectura literal: las relaciones se escriben en palabras, no como códigos. (4) Números y fechas: los umbrales ("al menos 20 millones de dólares"), los plazos y los conflictos los compara el **código**, no JEV. (5) Contenido adversarial: las vistas previas pasan por el mismo guardia contra instrucciones ocultas, porque un documento largo es más superficie de ataque.

---

## (d) Cambios concretos de código

| # | archivo / función | cambio | prioridad |
|---|---|---|---|
| 1 | `cmd/harness/abstain.go` (≈ l. 71–80) y `internal/judge/jevjudge.go` `EvidenceSufficiency` | **Bug:** se envía `n.Content` entero. Cortar con `truncate(..., probe.MaxPassageChars)` o, mejor, mandar hojas. En KoBLEX un artículo de 5.2M de caracteres entraría al estado. | ya |
| 2 | `internal/extract/legal/legal.go` | `CleanForJudge` (quitar etiquetas también del texto que lee JEV) y `Dedup` de ventanas repetidas (tejas de 200 caracteres; reporte con la razón de compresión). Bump de `EmbeddingRecipe` → `legal-v2`. | ya |
| 3 | nuevo `internal/ingest/tree.go` | `type Tree struct{ Doc string; Root *Unit }`, `type Unit struct{ Key, Path, Title, Lead, Text string; Level string; Start, End int; Kind string; Children []*Unit }`; `func Leaves(t *Tree, opt SizeOpts) []*Unit` (juntar, dividir, solapar, regla del encabezado). Genérico. | alta |
| 4 | nuevo `internal/ingest/profiles/{legal,markdown,html,plain}.go` | `type Profile interface{ Parse(docKey, title, text string) *Tree }`. Legal: regex de `(n)`, `n.`/`n-m.`, `(a)`, "Provided, That". Markdown: títulos `#`. HTML: `h1–h6`, `li`, `table`. Plano: párrafos → frases. | alta |
| 5 | `internal/ingest/nodes.go` | `func ToNodes(trees []*Tree) ([]*graphmodel.Node, []graphmodel.Edge)`: hojas y secciones como `Node` (`Label` = "Leaf"/"Section"; `Properties` doc/path/level/lead/char_start/char_end/kind); aristas `child_of`, `next`, `prev`, `references`, `defines`, `same_text`. | alta |
| 6 | `internal/extract/legal/legal.go` `ExtractCitations`/`BuildEdges` | Referencias con párrafo/numeral ("Article 34 (2) 1", "paragraph (3)", "subparagraph 5 of Article 2 of X Act"); resolver a la hoja más específica; "of the Act" en decretos → ley madre; tabla de definiciones (`"X" means`). | alta |
| 7 | `internal/notebook/sentences.go` `SplitSentences` | Perfil legal: también cortar en `;` + marcador de ítem, y antes de `\d+(-\d+)?\.` / `\([a-z]\)` pegados (hoy "…;2. A person…" queda en una sola frase); descartar etiquetas `<…>` como frase. Con hojas ≤ 2,500 caracteres, el tope de 25 frases y el corte a 6,000 de `sentencesOf` dejan de morder. | alta |
| 8 | `internal/notebook/silver.go` `Fact`, `judge.go` | Campos de procedencia (b.3); estados `ExpandState`, `ContinueState`, `SectionState`, `DefinitionState`; preguntas `ExpandQuestions`, `ContinueQuestions`, `SectionQuestions`, `DefinitionQuestions`, `unread_exception`. | alta |
| 9 | `internal/explorer/explorer.go` | `Explorer` recibe `Graph *ingest.Links` (hijos, hermanos, referencias, definiciones por llave). `Config` agrega `Expand bool; ExpandThreshold float64; MaxExpandPerRound, MaxPerDoc, MaxDepth, MaxChars int; TOC bool`. Nueva `expand.go`: `func (e *Explorer) candidates(h *Node, picked []factP) []cand` (código) y `func (e *Explorer) judgeExpansions(...)` (JEV). En `Explore`, después de elegir frases: los candidatos aceptados se agregan a `fresh` de esa ronda y se califican y leen. `seen` guarda `via` y `from`. `Trace` agrega `Expanded [][]string`, `CharsRead int`. | alta |
| 10 | `internal/explorer/explorer.go` `sentencesOf` / `truncate` | Si `len(Content) > MaxPassageChars` en una hoja, es un **error de ingesta**: registrar y contar, en vez de cortar en silencio. Sin hojas (modo actual), conservar el corte para comparar. | media |
| 11 | `cmd/harness/bench.go` | `benchmark` agrega `Chunking string` y `GoldSpans map[qid][]Span`. `loadKoBLEX` con `--chunking none|structure|structure+lead`; `Canon` colapsa llave de hoja → `ACT#art` para la métrica de documento. `DocText` = ruta + encabezado + hoja (cortado a 8,500, que ya no se alcanza). | alta |
| 12 | `internal/evaluator` | `SpanRecallAtBudget(gold []Span, ranked []Leaf, chars int)`, `ReadPrecision`, cortes por posición del oro. | alta |
| 13 | `cmd/harness/explore.go` | Variantes `--variants longdoc`: (a) nodo = documento cortado (hoy), (b) hojas sin expandir, (c) hojas + ruta, (d) hojas + ruta + expandir, (e) (d) + índice; reporte por posición del oro y caracteres leídos. | alta |
| 14 | nuevo `cmd/harness/longdoc.go` + `internal/datasets` | Cargadores: MuSiQue largo sintético (E1), ConditionalQA (vía DAPR en HF), QASPER, LegalBench-RAG-mini; mapeo de oro a rangos. | media |
| 15 | nuevo `cmd/harness/expandbank.go` | Banco de la pregunta `expand_useful` (E2), como `probebank.go`: AUC dentro de cada hoja de partida. | media |
| 16 | `internal/ingest/enrich.go` (opcional) | Contexto generado por LLM al ingerir, guardado en `Properties["context"]`, con caché por hash de documento. Solo para E6. | baja |

Todo con TDD como pide §9 del plan: pruebas de tabla para los perfiles (artículo con `(1)…(3)`, numerales `2-2.`, "Provided, That", tabla con encabezado repetido) y una prueba de propiedad: la concatenación de las hojas reproduce el texto normalizado sin huecos ni duplicados, salvo el solapamiento declarado.

---

## (e) Experimentos priorizados (con costo estimado)

Precios del repo: JEV US$0.042 por millón de tokens de entrada (`jevPricePerMTok`), Gemini embedding US$0.15 por millón (`embedPricePerMTok`), ~4.35 caracteres por token. Referencias: H6 costó ~US$0.19 por variante con JEV (150 preguntas); el embedding de MuSiQue ~US$0.065.

**E0 — Auditoría de corpus y oro (gratis, primero).**
Script local: distribución de longitudes, deduplicación de ventanas (cuántos artículos y caracteres se recuperan), número de hojas por perfil, porcentaje de oro de KoBLEX mapeado a hojas, y frases por hoja con el separador nuevo. Puerta: ≥ 98% del oro mapeado; 0 hojas > 2,500 caracteres sin justificación.

**E1 — "Documento largo sintético" con MuSiQue (principal; ~US$1).**
- Construcción: para cada pregunta de la memoria A (§25), cada párrafo oro se **entierra** en un documento de 8k–40k caracteres formado por párrafos distractores *del mismo título o temáticamente cercanos* (vecinos por embedding ya pagados), en una posición controlada (inicio / medio / final; > 6k obligatoria en dos tercios de los casos). Distractores no oro y misma regla anti-fuga de §20.
- Sistemas: (a) nodo = documento (embedding y JEV cortados, como hoy); (b) hojas = párrafos originales (**embeddings ya en caché, US$0**); (c) hojas + ruta; (d) hojas + expandir; (e) (d) con presupuesto igual de caracteres leídos.
- Métricas: cadena@10 a nivel de hoja y de documento, recall de rango @5k caracteres, por posición del oro y por saltos.
- Hipótesis pre-registrable: (d) − (a) en cadena@10 ≥ +0.15 en oro con posición > 6k; (d) − (b) ≥ 0 sin pérdida significativa en oro < 6k.
- Costo: Gemini solo para los embeddings de los documentos de (a), ~150 × 25k caracteres ≈ 0.9M tokens ≈ US$0.13. JEV ≈ 4 variantes × 150 preguntas × ~80 llamadas × ~700 tokens ≈ 34M tokens ≈ US$1.4. Con caché de P1/H6, menos.
- Por qué primero: aísla el efecto de "contenido largo" con datos y oro que ya entendemos.

**E2 — Banco de la pregunta de expansión (~US$0.10).**
Igual que P1/T2. Desde cada hoja oro del paso k (MuSiQue largo de E1, KoBLEX partido), se generan candidatos `next/prev/lead/ref/def` y se etiqueta como positivo el que contiene oro del mismo paso o del siguiente. Se compara: `expand_useful` de JEV vs "siempre next" vs similitud de embedding (pregunta + cuaderno, candidato) vs reordenador bge. Métrica: AUC dentro de cada hoja de partida, con IC por pregunta; y con cuaderno oráculo vs escogido por JEV vs equivocado. Puerta: AUC de JEV ≥ heurística + 0.05 y con hecho equivocado sin caída > 0.05. Si falla, la expansión se hace con reglas (siempre `lead` y `ref`) y JEV solo califica después, como pasaje.

**E3 — ConditionalQA (procedimientos, condiciones; ~US$1.5).**
652 documentos del gobierno del Reino Unido con secciones y listas; 271 preguntas de prueba en DAPR, con no contestables y respuestas condicionales. Es el dominio "empresa/procedimientos" que falta.
- Sistemas: (a) documento cortado; (b) hojas por sección/párrafo; (c) + ruta; (d) + expandir + `unread_exception`.
- Métricas: recall de evidencia @5k caracteres; cobertura de **condiciones** (fracción de condiciones oro cuya frase está en el cuaderno); AUROC de abstención.
- Costo: corpus ≈ 652 × ~10k caracteres ≈ 6.5M ≈ 1.5M tokens ≈ US$0.23 de embedding (más la ruta, que es corta); JEV ≈ 271 × 90 × 700 ≈ 17M ≈ US$0.7 por variante con JEV; dos variantes ≈ US$1.5.

**E4 — KoBLEX partido, no regresión contra H9 (~US$2.5).**
Todo el corpus partido por estructura: ~68M caracteres − deduplicación ≈ 45–50M ≈ 11M tokens ≈ **US$1.7** de embedding (solo los 742 largos: ≈ US$0.3, aunque así se mezclan granularidades). JEV ≈ test 113 preguntas × 2 variantes ≈ US$0.6. Hipótesis: R@5 y cadena@10 a nivel de artículo **no bajan** (no inferioridad, margen −0.02) y recall de rango @2k caracteres sube. Ojo: aquí el beneficio esperado es de **precisión** (citar el párrafo exacto), no de alcance. El problema de largo en KoBLEX afecta a ~13 contextos oro.

**E5 — QASPER y LegalBench-RAG-mini (~US$3–4, después).**
QASPER (secciones de artículos científicos; preguntas escritas sin ver el texto; evidencia por párrafo; no contestables) prueba el **modo índice**. LegalBench-RAG-mini (776 preguntas, oro por rango de caracteres, contratos de decenas de miles de caracteres) prueba fronteras de corte y recall de rango. Los autores reportan que el divisor recursivo sin reordenador fue lo mejor y que el reordenador Cohere empeoró: coherente con nuestro hallazgo con bge (§27–28). Costo dominado por el embedding de los contratos usados en mini (estimar en E0; orden US$1–2) + JEV ~US$1.5.

**E6 — Enriquecimiento con LLM al ingerir (~US$2–5, opcional, al final).**
Ruta de títulos (gratis) vs contexto de 50–100 tokens generado por un LLM pequeño con el documento en caché (Anthropic reporta ~US$1 por millón de tokens de documento con Haiku y caché de prompt). Solo en E1 y E3. Puerta: + ≥ 0.03 en recall de rango @5k sobre la ruta de títulos, o no se adopta (costo de ingesta, riesgo de alucinar contexto, contexto que se vuelve viejo cuando cambia el documento).

**E7 — Pruebas de estrés baratas (≤ US$0.3).**
(1) "Agujero negro": un documento de 40k caracteres casi relevante en todas partes; medir que D=8 y la parada local funcionen (caracteres leídos, otros documentos no desplazados). (2) Versiones casi duplicadas (ley enmendada): el cuaderno marca conflicto por número o fecha en código. (3) Inyección dentro de un documento largo en la hoja 30: que la vista previa no la ejecute ni suba su puntaje. (4) Referencia rota o ambigua ("the Act"): tasa de resolución errónea.

**Orden y costo total:** E0 (0) → E1 (~1) → E2 (~0.1) → [decidir] → E3 (~1.5) → E4 (~2.5) → E5 (~3–4) → E6/E7 (~3). Total ≈ **US$10–12**, todo con pre-registro y una sola corrida final por hipótesis, como en §22.3.

---

## (f) Riesgos

1. **Hojas sin contexto.** Un numeral sin su encabezado o un "dicho plazo" sin antecedente se recupera mal y JEV lo juzga mal. Mitigación: regla del encabezado, ruta de títulos, expansión `lead` gratis cuando la hoja empieza con marcador de ítem. Medir en E1/E3 por separado los oros que son "ítems de lista".
2. **Fragmentación de la cadena.** Partir convierte una pregunta de 1 artículo en una de 2–3 hojas, y cadena@k baja aunque el sistema "lea bien". Mitigación: reportar a ambos niveles; colapsar a documento para comparar con H6/H9; medir recall de rango con presupuesto en caracteres, no por k.
3. **Agujero negro y costo.** La expansión puede caminar un documento entero. Mitigación: D, profundidad, presupuesto de caracteres y parada local; registrar caracteres leídos por pregunta.
4. **Errores de JEV que se arrastran.** Un `expand_useful` falso positivo mete ruido al cuaderno. Mitigación: lo expandido pasa por las mismas preguntas de pasaje y de frase (doble filtro); confianza acotada por la del padre; banco E2 antes de integrar.
5. **Indirección de referencias.** "Art. 34 (2) 1 de la Ley" desde un decreto exige saber cuál es "la Ley": eso lo resuelve el código, no JEV. Si la resolución falla, la expansión trae el artículo equivocado con apariencia de autoridad. Mitigación: confianza de resolución en la arista; no expandir referencias ambiguas, solo mandarlas a BM25.
6. **Parser frágil por dominio.** Formatos sin marcadores claros (PDF escaneado, actas). Mitigación: perfil "plano" por frases como respaldo; el perfil se elige por documento y se reporta el porcentaje con respaldo.
7. **Basura de extracción.** Texto duplicado como el de KoBLEX, cuyo largo es falso, infla índices y costo. Mitigación: deduplicar en el paso 0 y alertar por razón de compresión.
8. **Resúmenes generados.** Si se adoptan (E6 o tipo RAPTOR), pueden inventar o envejecer. Regla: nunca se citan, se marcan con su origen y se regeneran por hash del documento.
9. **Contaminación de evaluación.** Los documentos sintéticos de E1 pueden ser demasiado fáciles (distractores de otro tema) o imposibles (distractores que contradicen). Mitigación: distractores del mismo título o vecinos por embedding, y registrar la mezcla.
10. **Regresión en lo corto.** MuSiQue ya es "una hoja por párrafo": el cambio no debe moverlo. Prueba: H6 con `--chunking structure` debe dar resultados idénticos (mismas llaves, mismos hashes de embedding).

---

## (g) Fuentes

- Anthropic, *Introducing Contextual Retrieval* (2024): https://www.anthropic.com/news/contextual-retrieval
- PageIndex (VectifyAI), índice en árbol y recuperación por razonamiento: https://github.com/VectifyAI/PageIndex
- Günther et al., *Late Chunking* (Jina, 2024): https://huggingface.co/papers/2409.04701
- Merola y Singh, *Reconstructing Context: Evaluating Advanced Chunking Strategies* (ECIR 2025 workshop): https://arxiv.org/abs/2504.19754 (resumen en https://alphaxiv.org/overview/2504.19754v1)
- Pipitone y Alami, *LegalBench-RAG* (2024): https://huggingface.co/papers/2408.10343
- Lee et al., *ReadAgent* (2024): https://huggingface.co/papers/2402.09727
- Sarthi et al., *RAPTOR* (2024): https://huggingface.co/papers/2401.18059
- Chen et al., *MemWalker* (2023): https://huggingface.co/papers/2310.05029
- Chen et al., *Dense X Retrieval: propositions* (2023): https://huggingface.co/papers/2312.06648
- Qu et al., *Is Semantic Chunking Worth the Computational Cost?* (2024): https://huggingface.co/papers/2410.13070
- Chroma Research, *Evaluating Chunking Strategies for Retrieval* (2024): https://research.trychroma.com/evaluating-chunking
- Bhat et al., *Rethinking Chunk Size for Long-Document Retrieval* (Fraunhofer, 2025): https://arxiv.org/abs/2505.21700
- NVIDIA, estudio de estrategias de chunking (página como unidad en PDFs financieros), resumido en: https://blockchain.news/news/optimizing-ai-retrieval-best-chunking-strategy
- Jiang et al., *LongRAG* (2024): https://huggingface.co/papers/2406.15319
- Liu et al., *Lost in the Middle* (2023): https://huggingface.co/papers/2307.03172
- LlamaIndex, *Auto-Merging Retriever* (jerarquía 2048/512/128): https://developers.llamaindex.ai/python/examples/retrievers/auto_merging_retriever/
- Docling, *Hybrid chunking* y `contextualize()`: https://docling-project.github.io/docling/_generated/examples/hybrid_chunking/
- OASIS LegalDocML / Akoma Ntoso, jerarquía `ANhier`: https://docs.oasis-open.org/legaldocml/akn-core/v1.0/csd01/part2-specs/material/AkomaNtoso30-csd13_xsd_Element_Group_ANhier.html
- DAPR (Document-Aware Passage Retrieval; incluye ConditionalQA, NQ): https://huggingface.co/datasets/UKPLab/dapr
- ConditionalQA (Sun et al.): https://huggingface.co/papers/2110.06884
- QASPER (Dasigi et al.): https://huggingface.co/papers/2105.03011.md y https://huggingface.co/datasets/allenai/qasper

*Nota: arxiv.org y aclanthology.org están bloqueados en esta red. Las cifras de los trabajos vienen de páginas de huggingface.co/papers, READMEs y blogs oficiales; las de arXiv 2504.19754 y 2505.21700 vienen de los resúmenes del buscador.*
