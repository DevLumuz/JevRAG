# Arnés de Evaluación: Grafo de Conocimiento + JEV

*Diseño genérico, para cualquier dominio de conocimiento — leyes es el primer dominio de prueba, no una limitación del diseño.*

## Índice

1. [Introducción y objetivo](#1-introducción-y-objetivo)
2. [Contexto: qué ya existe y qué estamos probando](#2-contexto-qué-ya-existe-y-qué-estamos-probando)
3. [Arquitectura general](#3-arquitectura-general)
4. [El grafo de conocimiento: nodos, conexiones y estructuras](#4-el-grafo-de-conocimiento-nodos-conexiones-y-estructuras)
5. [Stack técnico](#5-stack-técnico)
6. [Las cinco opciones a comparar y el diseño del Judge](#6-las-cinco-opciones-a-comparar-y-el-diseño-del-judge)
7. [Clientes externos: interfaces](#7-clientes-externos-interfaces)
8. [Estructura del proyecto](#8-estructura-del-proyecto)
9. [Metodología de desarrollo: TDD y calidad de código](#9-metodología-de-desarrollo-tdd-y-calidad-de-código)
10. [Métricas de evaluación](#10-métricas-de-evaluación)
11. [Datasets](#11-datasets)
12. [Criterios de decisión: ¿cuándo escalamos?](#12-criterios-de-decisión-cuándo-escalamos)
13. [Plan de construcción por fases](#13-plan-de-construcción-por-fases)
14. [División de trabajo](#14-división-de-trabajo)
15. [Riesgos a vigilar](#15-riesgos-a-vigilar)
16. [Glosario](#16-glosario)

---

## 1. Introducción y objetivo

Este documento define, de principio a fin, cómo vamos a determinar si construir un knowledge graph y usar **JEV** (el modelo de decisión de TypeSafe) como controlador de traversal mejora de verdad la calidad de la evidencia que le llega a un agente — frente a alternativas más simples.

Caso de uso guía para la primera prueba: un abogado con una base de conocimiento de leyes. Ante cada consulta, el sistema decide qué artículos son evidencia relevante para responder.

**El diseño en sí no está atado a leyes.** El núcleo del sistema (el grafo, el traversal, la interfaz Judge, el evaluador) es genérico y sirve para cualquier conocimiento que se quiera indexar — películas, documentación técnica, lo que el usuario decida cargar. Leyes es el primer dominio de prueba porque sus relaciones son explícitas y el riesgo de equivocarse es alto, no porque el sistema esté limitado a eso. La sección 4 explica cómo se logra esa generalidad.

**Objetivo explícito:** el arnés no parte de la premisa de que el grafo o JEV vayan a ganar. Es una herramienta de medición neutral. Concluir que la complejidad no se justifica es un resultado tan válido como el contrario.

Preguntas que el arnés debe contestar:

- ¿El grafo mejora recall sobre búsqueda vectorial plana?
- Si el grafo ayuda, ¿hace falta un judge por nodo, o basta con el traversal algorítmico puro (PPR)?
- ¿JEV iguala, en una fracción del costo, lo que logra un LLM grande haciendo de judge?
- ¿Cuál es el costo y la latencia real de cada arm?
- ¿El sistema abstiene correctamente cuando la evidencia es insuficiente?

---

## 2. Contexto: qué ya existe y qué estamos probando

- **GraphRAG** (Microsoft): construye un knowledge graph de entidades y resume comunidades relacionadas.
- **HippoRAG**: construye un grafo similar, pero explora usando Personalized PageRank (PPR) — un algoritmo determinista, sin ningún modelo de decisión en el camino de la consulta.
- **HippoRAG 2**: encontró que PPR puro se queda corto en recall factual, y lo corrigió agregando un filtro de reconocimiento con LLM sobre los candidatos que PPR prioriza. Es evidencia externa, publicada, de que grafo + judge le gana a grafo solo.
- **JEV** (TypeSafe): modelo no autoregresivo, devuelve decisiones tipadas (Choice / Score / Noul) en vez de texto, a una fracción del costo y la latencia de un LLM generativo.
- **CatRAG** (feb. 2026, sobre HippoRAG 2): identificó la "falacia del grafo estático" — los pesos fijos de un grafo no distinguen relación general de relevancia para una pregunta puntual — y la corrigió multiplicando el peso estático por un juicio de LLM específico a cada conexión y consulta. Es la validación más directa y más reciente de por qué el `Judge` de este documento evalúa conexiones, no nodos aislados.

Lo que ya está validado externamente: la arquitectura general (grafo + judge + evidence sufficiency + citación a la fuente) tiene fundamento, y GraphRAG/HippoRAG ya se usan sobre dominios genéricos, no solo texto legal. Lo que este arnés prueba específicamente es si **JEV** puede ocupar el rol de judge que hoy ocupa un LLM grande en HippoRAG 2, sin perder calidad de forma desproporcionada — probado primero en leyes, por ser un dominio de relaciones explícitas y alto costo de error.

---

## 3. Arquitectura general

```
Pregunta del usuario
        │
        ▼
¿Requiere retrieval? ──── No ──→ el agente responde directo
        │ Sí
        ▼
Clasificación de complejidad (lookup simple vs. multi-hop)
        │
        ▼
Retrieval de candidatos (con o sin grafo, según el arm)
        │
        ▼
Judge por candidato (ninguno / JEV / LLM, según el arm)
        │
        ▼
Respuesta con citación a los nodos usados + nivel de confianza,
o abstención explícita si la evidencia es insuficiente
```

Este documento cubre la etapa de retrieval + judge (sección 6). El routing inicial (¿buscar? ¿qué tan complejo?) ya se identificó como un buen caso de uso de JEV, pero es un arnés aparte.

---

## 4. El grafo de conocimiento: nodos, conexiones y estructuras

Una primera versión de este documento ataba el nodo al dominio legal (campos fijos como `Law`, `ArticleNumber`, `InForce`). Eso no generaliza: si el conocimiento a cargar mañana son películas, o cualquier otra cosa, esos campos no aplican y habría que reescribir el tipo cada vez.

La solución que usa la mayoría de los grafos de conocimiento reales (y el modelo detrás de bases de datos de grafos como Neo4j) es el **grafo de propiedades etiquetado** (labeled property graph): un nodo tiene una etiqueta (`Label`) que dice qué tipo de cosa es — `"LegalArticle"`, `"Movie"`, `"Person"`, lo que aplique — y un conjunto abierto de propiedades (`Properties`), pares clave-valor específicos de ese dominio. Las aristas funcionan igual: un `Type` de texto libre (`"modifica"`, `"actuó_en"`, `"es_secuela_de"`) en vez de un enum fijo pensado para un solo dominio.

Con este modelo, el núcleo del sistema — el grafo, el traversal, la interfaz `Judge`, el evaluador — queda completamente genérico y no cambia entre dominios. Lo único que cambia por dominio es la **capa de extracción**: el código que lee el contenido crudo y produce `Node`/`Edge`. Para leyes, esa extracción es barata (parsing de citas explícitas ya escritas en el texto). Para un dominio sin esa estructura explícita, normalmente hace falta un paso con LLM que infiera las relaciones — el mismo enfoque que usan GraphRAG y HippoRAG sobre texto genérico —, más caro y menos confiable que el parsing legal, pero el resto del sistema no se entera de la diferencia.

```go
package graphmodel

import "time"

// Node es un nodo genérico del grafo: cualquier chunk o documento, de cualquier dominio.
// Implementa graph.Node (gonum exige un método ID() int64).
type Node struct {
	NumericID   int64             // requerido por la interfaz graph.Node de gonum
	Key         string            // ID propio y estable, ej. "civil_code_art_40" o "movie_inception_2010"
	Content     string            // el texto/contenido completo de este nodo, sin resumir
	Label       string            // qué tipo de cosa es: "LegalArticle", "Movie", "Person"...
	Properties  map[string]string // campos específicos del dominio, ej. {"law": "Código Civil", "in_force": "true"}
	Embedding   []float64         // huella numérica del contenido; se calcula una sola vez, no depende de ninguna consulta
	LastUpdated time.Time
}

func (n *Node) ID() int64 { return n.NumericID }

// Edge es una arista dirigida entre dos Node.
type Edge struct {
	F, T       *Node
	Type       string            // nombre de la relación: "modifica", "actuó_en", "es_secuela_de"...
	Origin     string            // "extracted" (escrita literalmente en la fuente) o "inferred" (deducida) — patrón tomado de Graphify
	Weight     float64           // fuerza de la conexión, precalculada (ej. similitud entre los dos Content); no depende de ninguna consulta, igual que Embedding
	Properties map[string]string // detalles opcionales adicionales
}

func (e Edge) From() graph.Node { return e.F }
func (e Edge) To() graph.Node   { return e.T }
func (e Edge) ReversedEdge() graph.Edge {
	return Edge{F: e.T, T: e.F, Type: e.Type, Origin: e.Origin, Weight: e.Weight, Properties: e.Properties}
}

// BuildGraph arma un grafo dirigido de gonum a partir de nodos y aristas ya extraídos.
func BuildGraph(nodes []*Node, edges []Edge) *simple.DirectedGraph {
	g := simple.NewDirectedGraph()
	for _, n := range nodes {
		g.AddNode(n)
	}
	for _, e := range edges {
		g.SetEdge(e)
	}
	return g
}
```

**Qué se precalcula y qué no — la distinción correcta:** hay dos parejas distintas a comparar, y solo una de ellas es imposible de guardar de antemano.

- *Nodo-a-nodo* (`Embedding`, y ahora `Edge.Weight`/`Origin`): depende solo del contenido de A y B, nunca de una pregunta futura. Por eso sí se precalcula y se guarda — es la "fuerza" de la conexión que faltaba en la primera versión de este documento. `Origin` distingue si la conexión estaba escrita literalmente en la fuente ("extracted") o fue deducida ("inferred") — patrón tomado de Graphify (github.com/Graphify-Labs/graphify), que etiqueta cada arista igual y explícitamente no guarda embeddings ("no vector store"), apoyándose en cambio en esta fuerza/procedencia estructural más el grado de conexión de cada nodo.
- *Nodo-a-pregunta* (relevancia, la que decide un `Judge`): depende de la consulta de turno, cambia en cada llamada — nunca se guarda en el nodo ni en la arista, vive temporalmente en el `Decision` de la sección 6.

`Weight` es la señal que el traversal usa para decidir qué conexión seguir primero — pero **no debe ser la palabra final**. Un paper reciente (CatRAG, feb. 2026, construido sobre HippoRAG 2) le pone nombre a confiar solo en el peso estático: la "falacia del grafo estático" — dos nodos pueden estar fuertemente conectados en general sin que esa conexión importe para una pregunta específica, y un traversal que solo sigue `Weight` se desvía hacia nodos muy conectados pero irrelevantes para esa pregunta en particular, encontrando evidencia parcial pero perdiendo la cadena completa. Su solución, que adoptamos aquí: el peso que realmente guía el traversal para una consulta es `Weight × juicio del Judge para esa conexión específica` — ver la sección 6, donde el `Judge` ya no evalúa un nodo aislado, sino la conexión completa (de dónde se viene, hacia dónde se podría ir, y qué ya se confirmó).

**Idea para después, no para el MVP:** Graphify también registra (`graphify reflect`) si un nodo resultó útil en preguntas pasadas, y ajusta su confiabilidad con el tiempo — un tercer tipo de valor, ni fijo desde el inicio ni recalculado por completo cada vez, sino que se va actualizando con el uso. Vale la pena considerarlo una vez que el arnés básico funcione, pero agregarlo ahora complicaría la medición justo cuando el objetivo es aislar si JEV ayuda o no.

**Ejemplo de uso — mismo tipo, dos dominios distintos:**

```go
// Leyes:
legalNode := &graphmodel.Node{
	Key:     "civil_code_art_40",
	Content: "Artículo 40. ...",
	Label:   "LegalArticle",
	Properties: map[string]string{
		"law": "Código Civil", "article_number": "40", "in_force": "true",
	},
}

// El mismo tipo, con conocimiento de películas:
movieNode := &graphmodel.Node{
	Key:     "movie_inception_2010",
	Content: "Inception es una película de ciencia ficción...",
	Label:   "Movie",
	Properties: map[string]string{
		"director": "Christopher Nolan", "year": "2010",
	},
}
```

Ni `graphmodel.Node`, ni `Edge`, ni el traversal, ni el `Judge` cambian entre estos dos casos — solo cambia qué extractor de dominio los produce (sección 8).

**Nota de implementación:** verificar la firma exacta de `simple.DirectedGraph` / `graph.Node` / `graph.Edge` contra la documentación actual de `gonum.org/v1/gonum/graph` al momento de codificar — el snippet de arriba refleja el patrón esperado, no un copy-paste garantizado.

**Sobre Personalized PageRank:** el paquete `gonum.org/v1/gonum/graph/network` implementa PageRank estándar (teletransportación uniforme), **no** la variante *personalized* (con un vector de reinicio sesgado hacia los nodos semilla) que necesitamos para replicar el mecanismo de HippoRAG. Para eso, usar el paquete `centrality` de `github.com/intelligrit/graphwizard`, que sí expone Personalized PageRank construido sobre las interfaces de gonum — confirmar la firma exacta en su documentación antes de integrar.

---

## 5. Stack técnico

| Pieza | Elección | Nota |
|---|---|---|
| Lenguaje | Go | Compila a binario único, sin dependencias de servidor; buena concurrencia para lanzar evaluaciones de nodos en paralelo |
| Estructura de grafo | `gonum.org/v1/gonum/graph` + `graph/simple` | Genérica — el mismo tipo `Node`/`Edge` sirve para cualquier dominio (sección 4) |
| Personalized PageRank | `github.com/intelligrit/graphwizard` (paquete `centrality`) | gonum core no trae la variante personalizada — ver sección 4 |
| Embeddings | Gemini, `gemini-embedding-001` | Para retrieval inicial por similitud (seed nodes) |
| Judge a probar | JEV (TypeSafe) | Hipótesis central |
| Judge de referencia | Gemini (LLM generativo) | Techo de calidad, réplica del rol que cumple el LLM en HippoRAG 2 |
| Persistencia del grafo | Archivo JSON en disco, construido una vez | Sin base de datos ni servicios externos |
| Carga de datasets | HTTP directo a `datasets-server.huggingface.co` | JSON plano, sin dependencia de Python |

**Qué se construye vs. qué ya existe:** Gemini tiene SDK oficial en Go (`google.golang.org/genai`, mantenido por Google) — se usa directo, no se construye nada. JEV no tiene SDK oficial en Go todavía (solo Python/JS) — existen clientes de comunidad (`github.com/fgn/jevgo` es el más sólido visto hasta ahora), que se usan como transporte detrás de nuestra propia interfaz `jev.Client` (sección 7), nunca referenciados directo, precisamente porque son de comunidad y el producto es de días de nacido.

**Fuera de alcance para este arnés:** la generación de la respuesta final en prosa citada. Las métricas (recall@K, MRR, abstención) se calculan comparando los nodos aceptados contra el gold set de cada dataset — no requieren generar texto final. El LLM de la Opción 4 se usa solo como judge (clasifica candidatos), no como generador. Meter un paso de generación real mezclaría calidad de redacción con calidad de retrieval, justo lo que este diseño evita. Se agrega como validación aparte más adelante, una vez decidida qué opción de retrieval usar.

---

## 6. Las cinco opciones a comparar y el diseño del Judge

| # | Nombre | ¿Usa grafo? | Judge | Costo esperado | Qué mide |
|---|---|---|---|---|---|
| 1 | Vector RAG plano | No | Ninguno | Muy bajo | Piso de referencia |
| 2 | Vector RAG + JEV | No | JEV | Bajo | Si JEV ayuda incluso sin grafo |
| 3 | Grafo + PPR puro | Sí | Ninguno | Muy bajo | Si el grafo solo, sin judge, ya alcanza |
| 4 | Grafo + LLM (Gemini) | Sí | Gemini | Alto | Techo de calidad (réplica de HippoRAG 2) |
| 5 | Grafo + JEV | Sí | JEV | Bajo | **Hipótesis principal** |

La comparación central es **4 vs. 5**: mismo grafo, mismo PPR, la única variable es el judge. Este diseño (y el `Judge` de abajo) es igual de válido corriendo sobre el grafo legal que sobre cualquier otro dominio que use el mismo `graphmodel.Node`.

El diseño usa una interfaz común para que el mismo motor de traversal corra en las opciones 2, 4 y 5, cambiando solo el judge:

```go
package judge

import (
	"context"

	"yourmodule/internal/graphmodel"
)

// RelevanceTier son los niveles discretos en los que el Judge clasifica una CONEXIÓN
// específica (no un nodo aislado) — diseño validado por CatRAG (Lau et al., 2026) sobre HippoRAG 2.
type RelevanceTier int

const (
	Irrelevant RelevanceTier = iota // la conexión no aporta nada a esta pregunta
	Weak                            // vínculo válido, pero tangencial
	High                            // paso crítico en la cadena de razonamiento
	Direct                          // el nodo destino ES la respuesta o la contiene
)

// Multiplier traduce el nivel a un factor sobre el Weight estático de la conexión.
// Suprime lo irrelevante, amplifica lo directo — mismo mapeo no lineal que usa CatRAG.
func (t RelevanceTier) Multiplier() float64 {
	switch t {
	case Irrelevant:
		return 0
	case Weak:
		return 0.25
	case High:
		return 2.5
	case Direct:
		return 5.0
	}
	return 0
}

// Decision es el resultado de evaluar una conexión específica contra la consulta.
type Decision struct {
	Tier       RelevanceTier
	Confidence float64 // 0-1, calibrada
	Sufficient bool    // si, junto con lo ya recolectado, esto ya es evidencia suficiente
}

// Judge evalúa una conexión, no un nodo aislado: de dónde se viene, hacia dónde se podría
// ir, contra la consulta y contra lo ya confirmado. Implementaciones: JEVJudge, LLMJudge.
// La opción 3 (PPR puro) no usa Judge — es un algoritmo distinto, no una implementación vacía.
type Judge interface {
	ScoreEdge(ctx context.Context, edge graphmodel.Edge, query string, confirmed []*graphmodel.Node) (Decision, error)
}

// EffectiveWeight combina la fuerza estática de la conexión (fija, del grafo) con el
// juicio del Judge para esta consulta (dinámico) — ninguno reemplaza al otro.
func EffectiveWeight(edge graphmodel.Edge, d Decision) float64 {
	return edge.Weight * d.Tier.Multiplier()
}
```

`JEVJudge` traduce cada `ScoreEdge` en una llamada `Choice` a JEV (sección 7) con las cuatro opciones de `RelevanceTier`, pasando `edge.T.Content` (el candidato) y un resumen de `confirmed`. `LLMJudge` usa el mismo contrato con Gemini — es la implementación que aproxima el rol del LLM en HippoRAG 2/CatRAG, sin reimplementar toda su maquinaria de indexado. **Regla de contradicción** (tomada de CatRAG tal cual): si el candidato contradice algo en `confirmed`, el Judge regresa `Irrelevant` sin importar qué tan alto sea `edge.Weight`. Ninguna implementación necesita saber si `edge.T.Label` es `"LegalArticle"` o `"Movie"` — leen `Content` y deciden.

---

## 7. Clientes externos: interfaces

```go
package jev

import "context"

// Client cubre los tres primitivos de JEV.
type Client interface {
	Noul(ctx context.Context, state, question string) (probability float64, err error)
	Score(ctx context.Context, state, question string, levels []string) (score, confidence float64, err error)
	Choice(ctx context.Context, state, question string, options []string) (choice string, confidence float64, err error)
}
```

```go
package embeddings

import "context"

type Client interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}
```

```go
package datasets

import "context"

type Row struct {
	Index int
	Data  map[string]any
}

// Client habla con datasets-server.huggingface.co (endpoint /rows).
type Client interface {
	Rows(ctx context.Context, dataset, config, split string, offset, length int) ([]Row, error)
}
```

---

## 8. Estructura del proyecto

```
/cmd/harness             → CLI: --mode=dev|test --option=1..5 --dataset=koblex
/internal/graphmodel     → Node, Edge, construcción del grafo (genérico, cualquier dominio)
/internal/extract/legal  → parser de citas legales → produce []graphmodel.Node y []graphmodel.Edge
/internal/judge          → interfaz Judge, JEVJudge, LLMJudge
/internal/jev            → cliente de JEV
/internal/embeddings     → cliente de embeddings de Gemini
/internal/genai          → cliente de generación de Gemini (para LLMJudge)
/internal/datasets       → cliente del Datasets Server de Hugging Face
/internal/evaluator      → recall@K, MRR, abstención, costo, latencia
/testdata                → fixtures: nodos y preguntas de ejemplo con respuesta conocida
```

Un dominio nuevo (películas, lo que sea) se agrega como un paquete más bajo `/internal/extract/<dominio>` que sabe leer ese contenido y producir `graphmodel.Node`/`Edge` — todo lo demás en este árbol se reutiliza sin cambios.

---

## 9. Metodología de desarrollo: TDD y calidad de código

Requisito explícito: desarrollo guiado por pruebas (TDD), priorizando código simple y sólido sobre soluciones "clever".

- **Ciclo TDD por componente**: para cada pieza (`evaluator`, cada `Judge`, cada cliente, cada extractor de dominio), primero se escribe la prueba que describe el comportamiento esperado, luego el código mínimo que la pasa, luego se refactoriza.
- **La interfaz `Judge` existe también para esto**: permite probar el motor de traversal con un judge falso, sin llamar a JEV ni a Gemini en cada corrida de pruebas:

```go
type FakeJudge struct {
	Decisions map[string]judge.Decision // por Key del nodo destino, para pruebas deterministas
}

func (f *FakeJudge) ScoreEdge(ctx context.Context, edge graphmodel.Edge, query string, confirmed []*graphmodel.Node) (judge.Decision, error) {
	return f.Decisions[edge.T.Key], nil
}
```

- **Table-driven tests** (patrón estándar en Go) para todo lo que sea una función pura, especialmente el evaluador:

```go
func TestRecallAtK(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		got      []string
		k        int
		want     float64
	}{
		{"todo encontrado", []string{"art_40", "art_41"}, []string{"art_40", "art_41", "art_50"}, 3, 1.0},
		{"nada encontrado", []string{"art_40"}, []string{"art_99"}, 1, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecallAtK(tt.expected, tt.got, tt.k)
			if got != tt.want {
				t.Errorf("RecallAtK() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- **Simplicidad por encima de flexibilidad especulativa**: funciones cortas, un paquete con una responsabilidad, sin abstracciones para casos que todavía no existen. `Properties map[string]string` (en vez de tipos por dominio) es exactamente esa elección: simple y suficiente hasta que se demuestre que no alcanza.
- **Cobertura esperada**: cada paquete bajo `/internal` debe tener pruebas antes de darse por terminado. `/cmd` es orquestación de piezas ya probadas — necesita menos cobertura directa.

---

## 10. Métricas de evaluación

- **Recall@K**: de los nodos que hacían falta, cuántos aparecen entre los primeros K resultados. La métrica más importante para leyes.
- **MRR**: qué tan arriba aparece el primer nodo correcto, en promedio.
- **Exactitud de abstención**: si el sistema dice "evidencia insuficiente" cuando corresponde, y no lo dice cuando sí tenía lo necesario.
- **Costo por consulta** y **latencia por consulta**, por arm.

Disciplina de medición: split dev (para fijar umbrales) y split test (no se toca hasta que los umbrales estén congelados).

---

## 11. Datasets

- **Fase 1 — KoBLEX**, en tres repos separados de Hugging Face (vía Datasets Server):
  - `JihyungL/KoBLEX-koblex` — 226 preguntas + respuesta + provisiones correctas ya anotadas por expertos. Esto es lo único "ya hecho" — no incluye ningún grafo de citas.
  - `JihyungL/KoBLEX-statute-eng` — el corpus de artículos (inglés) del que construimos los `Node`. ~22 provisiones son traducción automática, marcadas como `MACHINE_TRANSLATED` en el dataset.
  - El grafo de conexiones (`Edge`) se construye nosotros, parseando citas explícitas en `KoBLEX-statute-eng` — no viene incluido.
  - No hay split dev/test provisto — se parte el bloque de 226 nosotros antes de fijar umbrales.
- **Fase 2 (condicional) — Bar Exam QA / Housing Statute QA** (Stanford RegLab; Bar Exam QA también empaquetado en `isaacus/mteb-barexam-qa` en Hugging Face, dentro de MLEB): ~2,000 pares con pasajes dorados anotados, sobre un pool de ~900K pasajes.

Ambos están en inglés, de jurisdicciones específicas. Sirven para validar la arquitectura genérica; un producto final en español, o en otro dominio, necesitará su propio corpus y su propio extractor (sección 8) más adelante.

---

## 12. Criterios de decisión: ¿cuándo escalamos?

Para pasar de KoBLEX a Bar Exam QA/Housing Statute QA, la opción 5 debe cumplir, frente a la opción 3, **las tres condiciones**:

1. Recall@5 al menos **15 puntos porcentuales** por encima
2. Costo no mayor a **2-3x** el de la opción 1
3. Abstención correcta **≥85-90%**

Medido sobre el split de test, con umbrales ya congelados en dev.

---

## 13. Plan de construcción por fases

1. Cliente de datasets (KoBLEX vía HF).
2. `graphmodel` genérico + `extract/legal` (parser de citas) + persistencia a JSON.
3. Opciones 1 y 2 (sin grafo) — las más baratas, primero.
4. Opción 3 (grafo + PPR) — reutiliza el grafo, sin judge.
5. Opciones 4 y 5 (grafo + judge) — reutilizan todo lo anterior, cambia el `Judge`.
6. `evaluator` — en paralelo a los pasos anteriores.
7. Corrida completa sobre KoBLEX, evaluación contra la sección 12.
8. Si aplica, repetir sobre Bar Exam QA / Housing Statute QA.

---

## 14. División de trabajo

| Tarea | Quién | Por qué |
|---|---|---|
| Cliente de HF Datasets Server | Con IA | Mecánico, API documentada |
| Cliente de JEV | Con IA — el acceso (lista de espera) es tuyo | Código mecánico; la cuenta no |
| Cliente de embeddings/Gemini | Con IA | Mecánico |
| `graphmodel` genérico sobre gonum/GraphWizard | Con IA | Uso de librerías ya existentes |
| `extract/legal` (parser de citas) | **Tuya**, de cerca | Un error aquí rompe el grafo en silencio — hace falta validar contra el texto real |
| `evaluator` | Con IA | Fórmulas bien definidas, buen candidato para TDD estricto |
| Ajuste de umbrales en dev | **Tuya** | Decisión de criterio |
| Decisión de escalar o no | **Tuya** | Decisión central del proyecto |

---

## 15. Riesgos a vigilar

- **Poda prematura**: descartar una rama demasiado rápido puede perder el nodo correcto. Ante la duda, revisar de más.
- **Confianza de JEV ≠ vigencia de la información**: alta confianza solo refleja certeza sobre el contenido que JEV vio, no que la base esté actualizada. Por eso `LastUpdated` va separado, en cualquier dominio.
- **Contenido adversarial**: texto con instrucciones ocultas puede manipular a un judge (documentado por TypeSafe mismo). No depender de un único filtro.
- **JEV es muy reciente**: lanzado hace pocos días, en lista de espera, con precios/límites que su creador dice que pueden cambiar.
- **Techo de recall antes de que JEV entre en juego**: JEV solo juzga lo que ya llegó como candidato. Un nodo con baja similitud de embedding y sin conexión (ni indirecta) a ningún punto de entrada nunca llega a que JEV lo vea, sin importar qué tan bueno sea su juicio — un detalle específico enterrado en un artículo mayormente sobre otro tema puede diluirse en el embedding de todo el documento. Mitigación, no solución completa: dado que JEV es barato, el filtro inicial por embeddings debe ser generoso (más candidatos entran a que JEV los revise, uno por llamada — nunca todos en un solo estado, por el "context rot" que TypeSafe documenta), y conviene correr búsqueda léxica (BM25) en paralelo a los embeddings como entrada adicional, ya que detecta coincidencias exactas de términos que la similitud semántica puede pasar por alto.

---

## 16. Glosario

- **Grafo de propiedades etiquetado**: modelo donde cada nodo/arista tiene una etiqueta (tipo) y un conjunto abierto de propiedades — la forma estándar de modelar cualquier dominio en un mismo grafo.
- **PPR (Personalized PageRank)**: variante de PageRank con un vector de reinicio sesgado hacia nodos semilla, en vez de teletransportación uniforme.
- **Judge**: componente que evalúa una conexión específica (no un nodo aislado) y decide su nivel de relevancia para la consulta actual.
- **Falacia del grafo estático**: confiar solo en pesos de conexión fijos para navegar, ignorando que la relevancia real depende de la pregunta puntual (CatRAG, 2026).
- **Recall@K**: proporción de resultados correctos encontrados entre los primeros K.
- **MRR**: posición promedio (invertida) del primer resultado correcto.
- **Abstención**: negarse a responder cuando la evidencia es insuficiente, en vez de alucinar.