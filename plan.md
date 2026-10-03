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
17. [Hipótesis de complejidad (pre-registro)](#17-hipótesis-de-complejidad-pre-registro)

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

// Client refleja la API oficial (POST https://api.typesafe.ai/v1/systemone):
// un solo request lleva el state y varias preguntas con nombre (choice/score/noul),
// que JEV contesta en paralelo. La respuesta trae answers por nombre + usage (tokens).
type Client interface {
	SystemOne(ctx context.Context, state any, questions map[string]Question) (*Response, error)
}
```

Las preguntas se construyen con `jev.Choice(instrucciones, criterios)`, `jev.Score(instrucciones, niveles)` y `jev.Noul(instrucciones)`. Clave en `TYPESAFE_API_KEY`; modelo por defecto `jev-latest`. Contrato verificado contra el SDK oficial de Python (`typesafe-sdk`, generado del `openapi.json` de la API).

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

---

## 17. Hipótesis de complejidad (pre-registro)

*Escrita el 3 de octubre de 2026, después de correr las opciones 1 y 2 en dev y **antes** de construir las opciones 3 y 5 y de tocar el split de test. Este texto no se edita después de ver test; cualquier cambio posterior va en una sección nueva, fechada.*

**Lo observado en dev (70 preguntas, exploratorio):** la opción 2 (JEV juzgando 30 candidatos por separado) sube Recall@5 de 0.783 a 0.879 frente a la opción 1. Por número de artículos necesarios: 1 artículo 1.00 → 1.00 (sin margen), 2 artículos 0.71 → 0.86 (+0.15, IC95 +0.075..+0.237), 3 artículos 0.74 → 0.79 (+0.05, IC95 0..+0.12). En 3 artículos el correcto estaba entre los 30 candidatos el 88% de las veces, pero JEV cubrió solo un tercio del hueco. Lectura: juzgar cada candidato aislado no reconoce los artículos "puente" de una cadena.

**H1 — JEV como filtro (opción 2 vs. 1).** Cuando el artículo correcto está entre los candidatos, JEV lo sube. Métrica: ΔRecall@5 pareado por pregunta y "hueco cubierto" = (R@5 opción 2 − R@5 opción 1) / (alcanzable@30 − R@5 opción 1).

**H2 — JEV navegando (opción 5 vs. 2 y vs. 3).** Con contexto de la cadena (de qué artículo viene la conexión y qué ya se confirmó), JEV recupera artículos puente que no reconoce aislados. Predicción: la ventaja de la opción 5 sobre la 2 **crece con el número de artículos necesarios** (1 < 2 < 3), y es mayor en preguntas cuyos artículos correctos están conectados por citas en el grafo.

**Métricas por grupo (1, 2, 3 artículos):** Recall@5, Recall@10, cadena completa@10 (todos los correctos en el top 10), MRR, llamadas al juez y costo. Diferencias pareadas por pregunta con IC95 por bootstrap; tendencia entre grupos con prueba de permutación sobre la pendiente de Δ contra número de artículos.

**Controles:** techo (1 artículo ya está en 1.00 con la opción 1), espacio (3 artículos compiten por 5 lugares; por eso también cadena completa@10), alcance (separar lo que estaba entre los candidatos de lo que no).

**Regla de decisión (en test, una sola corrida, configuración congelada en dev y commiteada antes):**
- H2 se apoya si Δ(opción 5 − opción 2) en Recall@10 es mayor en 3 artículos que en 1 artículo, la pendiente es positiva con p < 0.05 en la prueba de permutación, y el IC95 de Δ en 3 artículos excluye 0.
- H2 se rechaza si la pendiente es ≤ 0 o el IC95 de Δ en 3 artículos incluye 0. Ambos resultados se reportan igual.
- El criterio de escalar de la sección 12 (opción 5 vs. opción 3) se evalúa aparte, sin cambios.

**Diseño fijado de la opción 5 (antes de verla correr):** mismo grafo y mismo Personalized PageRank que la opción 3; el juez (1) califica las semillas, que pesan en el reinicio del PPR como similitud × multiplicador, y (2) califica conexiones desde los artículos aceptados hacia sus vecinos, con contexto de origen y de lo confirmado, hasta 2 saltos y un tope de llamadas por pregunta; cada conexión juzgada pesa Weight × multiplicador (CatRAG). Las conexiones no juzgadas conservan su peso. La opción 3 es el mismo código sin juez.


---

## 18. Configuración congelada para test (3 de octubre de 2026)

*Fijada en dev y commiteada **antes** de la primera corrida sobre el split de test. Es la configuración por defecto de `cmd/harness` en este commit.*

| Parámetro | Valor | Origen |
|---|---|---|
| Embeddings | gemini-embedding-001, 3,072 dims, receta legal-v1, preguntas RETRIEVAL_QUERY con contexto + pregunta | sección 2 de la corrida de embeddings |
| K reportado | 1, 5, 10 | plan |
| Opción 2 | 30 candidatos, umbrales del juez 0.5 | sin ajustar |
| Opciones 3 y 5 | 30 semillas, `seed-temp` 0.05, damping 0.3, peso de conexión = coseno entre artículos | ajustado en dev con la opción 3 (sin juez) |
| Opción 5 | 2 saltos, 5 vecinos por nodo aceptado, máx. 60 llamadas por pregunta, conexiones no juzgadas ×1 | dev: 30 semillas igualan a la opción 2; la variante ×0.25 no mejoró |

Corridas en test, una vez cada una: opciones 1, 2, 3 y 5. El análisis sigue la regla de la sección 17 sin cambios.

---

## 19. Resultados en test (3 de octubre de 2026)

*Una sola corrida por opción con la configuración de la sección 18. Análisis exacto de la sección 17.*

| Opción | R@1 | R@5 | R@10 | Cadena completa@10 | MRR | JEV (tokens · US$) |
|---|---|---|---|---|---|---|
| 1 vector | 0.459 | 0.795 | 0.860 | 0.724 | 0.799 | — |
| 2 vector + JEV | 0.526 | **0.858** | **0.915** | **0.833** | 0.853 | 4.29M · 0.18 |
| 3 grafo + PPR | 0.465 | 0.767 | 0.850 | 0.712 | 0.796 | — |
| 5 grafo + JEV | **0.542** | 0.845 | 0.902 | 0.814 | **0.868** | 9.88M · 0.41 |

**H1 (JEV como filtro) — se apoya.** ΔR@5 opción 2 − 1 = +0.063 (IC95 +0.029..+0.099). Por artículos: 1 → +0.037, 2 → +0.042, 3 → **+0.167 (IC95 +0.077..+0.256; 14 mejor, 2 peor)**. El patrón por artículos es exploratorio (no pre-registrado como pendiente) y en dev fue distinto (2 artículos +0.15, 3 artículos +0.05).

**H2 (JEV navegando) — se rechaza.** ΔR@10 opción 5 − 2: 1 art. +0.012, 2 art. −0.021, 3 art. −0.024; pendiente −0.020, p = 0.90; el IC95 en 3 artículos incluye 0. Cadena completa@10: misma dirección (pendiente −0.033, p = 0.89). Navegar no supera a juzgar la lista de candidatos y cuesta 2.3×.

**Criterio de escalar (sección 12) — no se cumple.** R@5 opción 5 − 3 = +0.078 (IC95 +0.044..+0.114), por debajo del umbral +0.15. Abstención: no medible en KoBLEX (no hay preguntas sin respuesta).

**Lectura:** en KoBLEX el valor de JEV está en juzgar candidatos (barato: US$0.0012 por pregunta) y crece en las preguntas de 3 artículos; el grafo de citas con PPR no supera a la búsqueda vectorial, con o sin juez. Siguiente prueba: dominio sin citas explícitas y con preguntas sin respuesta (MuSiQue).

---

## 20. Segundo dominio: memoria sin citas (MuSiQue) — pre-registro y configuración congelada (3 de octubre de 2026)

*Escrito después de correr dev (82 preguntas: 34 con respuesta, 48 sin respuesta) y antes de tocar test (218 preguntas).*

**Memoria:** 4,757 párrafos de Wikipedia de 150 preguntas con respuesta + 150 sin respuesta (MuSiQue-Full, muestreadas por hash; una pregunta sin respuesta solo entra si el párrafo que le quitaron no está en la memoria compartida). Conexiones inferidas, sin citas: menciones de título (frase completa) + 5 vecinos más similares. Texto embebido: título — párrafo.

**Lo observado en dev (exploratorio):** R@10 / cadena completa@10 — opción 1 0.819 / 0.588; opción 2 0.657 / 0.412; opción 3 0.833 / 0.618; opción 5 0.777 / 0.618. De los párrafos correctos que la opción 1 tenía en su top 10, la opción 2 descartó 15 de 47 "puente" (pasos intermedios) y 4 de 27 "finales"; la opción 5 conservó 36 de 47 puente. El filtro de suficiencia (JEV, 5 mejores pasajes) separa con AUC 0.65–0.77; mejor exactitud de abstención 0.71–0.73.

**H3 — juzgar aislado pierde los puentes.** En test, R@10 de la opción 2 < R@10 de la opción 1 (IC95 de la diferencia pareada excluye 0).

**H4 — navegar con contexto los recupera.** En test, cadena completa@10 de la opción 5 > opción 2 (IC95 excluye 0). Secundario: la diferencia por número de pasos (2, 3, 4), reportada sin prueba formal por el tamaño de los grupos.

**H5 — saber abstenerse.** Exactitud de abstención en test con el filtro de suficiencia; meta del plan (sección 12): ≥ 0.85. Se reporta para cada opción.

**Configuración congelada:** la misma de la sección 18 (30 semillas/candidatos, seed-temp 0.05, damping 0.3, 2 saltos, 5 vecinos, 60 llamadas). Filtro de suficiencia: 5 pasajes; umbral elegido en dev por exactitud balanceada: opción 1 → 0.38, opción 2 → 0.66, opción 3 → 0.29, opción 5 → 0.52.

---

## 21. Resultados en test — MuSiQue (3 de octubre de 2026)

*218 preguntas (116 con respuesta, 102 sin respuesta); una corrida por opción con la configuración de la sección 20.*

| Opción | R@5 | R@10 | Cadena completa@10 | MRR | Abstención | JEV US$ |
|---|---|---|---|---|---|---|
| 1 vector | **0.759** | 0.823 | 0.612 | 0.930 | **0.693** | 0.02 (filtro) |
| 2 vector + JEV | 0.628 | 0.694 | 0.422 | 0.764 | 0.619 | 0.22 |
| 3 grafo inferido + PPR | 0.754 | **0.831** | **0.621** | **0.932** | 0.688 | 0.02 (filtro) |
| 5 grafo + JEV navegando | 0.678 | 0.750 | 0.526 | 0.828 | 0.651 | 0.48 |

**H3 — se apoya.** R@10 opción 2 − 1 = −0.129 (IC95 −0.174..−0.084; 6 mejor, 40 peor). Juzgar cada párrafo aislado descarta evidencia puente.
**H4 — se apoya.** Cadena completa@10 opción 5 − 2 = +0.103 (IC95 +0.034..+0.172; 15 mejor, 3 peor). Por pasos: 2 → +0.143, 3 → +0.032, 4 → +0.067.
**Pero** ninguna opción con JEV supera a la búsqueda sin juez: opción 5 − 1 en cadena completa@10 = −0.086 (IC95 −0.164..−0.009).
**H5 — no se cumple.** Abstención 0.62–0.69, lejos de 0.85. El filtro de suficiencia separa (AUC 0.75–0.80) pero no lo bastante.

**Lectura:** en una memoria sin citas, el juez tal como está diseñado (descartar lo que juzga irrelevante, pregunta por pregunta completa) hace daño; dar contexto de la cadena reduce ese daño sin eliminarlo. El grafo inferido sin juez empata con la búsqueda vectorial. Mejoras candidatas, a evaluar solo en dev: que el juez reordene sin descartar, que juzgue contra sub-preguntas (descomposición) en lugar de la pregunta completa, y un filtro de suficiencia con más pasajes.

---

## 22. Plan v2: explorador con cuaderno de evidencia (3 de octubre de 2026)

*Escrito tras tres revisiones independientes (auditoría del banco de pruebas, revisión estratégica, catálogo de arquitecturas) y aprobado para ejecutar.*

### 22.1 Correcciones a lo anterior
- **Bug corregido:** las opciones de los `choice` y los criterios de los `noul` llegaban a JEV en orden alfabético (mapas de Go). Ahora se envían en el orden escrito (`jev.Ordered`). Todo resultado previo con `choice` está confundido con ese orden.
- **El "compuesto + rango" del banco (0.81–0.88) era un artefacto** del banco (marca −100 solo en oro no recuperado; negativos limitados a rangos 1–7). Sin él no hay ganancia. v1 leído como 1 − P(irrelevant) empata con los conjuntos nuevos.
- **Métrica correcta del banco:** ordenar los pasajes de *una misma* pregunta (AUC dentro de la pregunta, contra el orden de embeddings, con IC por pregunta). Así medido, JEV gana claramente en KoBLEX y no significativamente en MuSiQue.
- **El fallo real son los puentes de paso intermedio:** 9 de 25 nunca llegan al top 30; los demás JEV los separa mal sin contexto.
- **La abstención medida contra "lo que la búsqueda trajo" fue 0.85;** el límite era la recuperación.
- **Nunca se comparó JEV contra un reordenador estándar** (bge-reranker, etc.). Es obligatorio antes de afirmar que JEV es la pieza clave.

### 22.2 Arquitectura v2
Rondas de exploración con un **cuaderno de evidencia**. JEV nunca escribe; escoge, califica y decide:
1. Búsqueda híbrida (embeddings + BM25) → JEV califica pasajes (**reordena, nunca descarta**).
2. JEV **escoge frases clave** de los pasajes → cuaderno `{frase textual, fuente, ronda, padre, entidades, requisito que cubre, confianza}`.
3. Cobertura por requisito sobre el cuaderno → parar, seguir o abstenerse.
4. Rondas siguientes: búsquedas nuevas desde el cuaderno (pregunta + hechos re-embebidos, BM25 de entidades nuevas, vecinos del grafo); JEV juzga con una vista corta del cuaderno (≤ 8 hechos + 1–3 entidades frontera).
5. Salida para el agente: el cuaderno (fragmentos con cita, cobertura, huecos, conflictos).

Contra errores que se arrastran: confianza acotada por la del padre, haz de 2 ramas ante duda, poda de ramas estériles, contradicciones marcadas (números y fechas comparados en código), bono por corroboración, guardia contra instrucciones ocultas.

### 22.3 Fases (cada una: pre-registro fechado, ajuste en dev, congelar, una corrida final)
| Fase | Qué | Puerta |
|---|---|---|
| 0 | Banco v2 (negativos de todo el top 30, sin artefacto de rango, etiquetas "relacionado", puente primer paso / intermedio, AUC por pregunta con IC); dev nuevo de MuSiQue (train); BM25 + híbrido; alcance@30/100/300 de puentes intermedios | Línea base honesta |
| P1 | Pruebas del cuaderno: T1 alcance con hechos oráculo; T2 reconocimiento sin cuaderno / oráculo / hecho equivocado / escogido por JEV; T3 selección de frase clave | Oráculo sube AUC de puentes intermedios ≥ 0.10 y el hecho equivocado no la baja > 0.05 |
| 1 | JEV contra reordenador estándar (bge-reranker-v2-m3) y combinados | Si el reordenador iguala a JEV (±1 pt) y JEV no suma ≥ 0.02 AUC, JEV deja de ordenar |
| 2 | Bucle de 2–3 rondas con cuaderno vs. una pasada con el mismo presupuesto | Cadena completa@10 ≥ control + 0.05 (IC excluye 0) |
| 3 | Cobertura por requisito y abstención (oráculo / LLM pequeño / plantilla) vs. filtro actual y juez LLM | AUROC ≥ 0.85, exactitud balanceada ≥ 0.80 |
| 4 | Prueba de producto: lector fijo, verificación de citas, español, inyección; ConditionalQA / LegalBench-RAG / MultiHop-RAG; conjunto sellado | Sí/no al producto |

---

## 23. Resultados P1 — pruebas del cuaderno en MuSiQue (3 de octubre de 2026)

*Prueba de mecanismo sobre las 150 preguntas con respuesta de la memoria MuSiQue (incluye preguntas de dev y test de §20–21; no es evaluación final). Hechos "plata": la frase del párrafo oro de cada paso que contiene la respuesta de ese paso (382 de 386 exactas). Reportes: `results/probe/notebook-T{1,2,3}-musique.md`. Costo: Gemini ~US$0.005, JEV ~US$0.21.*

**T1 — alcance (¿la siguiente búsqueda encuentra el párrafo del paso siguiente?).** Pasos intermedios (n = 63), alcance@30: pregunta sola (hoy) 0.62 → pregunta + hechos del cuaderno 0.90 → palabras clave de la entidad frontera 0.94 (techo con sub-pregunta resuelta: 0.94). Pasos finales: 0.88 → 0.98. BM25 con la pregunta sola es peor que vectores (0.33); el híbrido sin cuaderno no ayuda (0.51).

**T3 — ¿JEV escoge la frase clave?** Top-1 contra la frase plata (386 párrafos, 3.6 frases de media):
primera frase 0.68 · solapamiento de palabras 0.60–0.66 · **JEV choice 0.89** · JEV sí/no por frase 0.87. Pasos intermedios: 0.56 / 0.54 / **0.79** / 0.75 (top-2: 0.92 / 0.97). Párrafos donde la frase clave no es la primera: solapamiento 0.44, **JEV 0.84**. Problema: el choice eligió "none" en 70 de 386 (todos tenían el hecho) → en el bucle usar sí/no por frase o el máximo sin "none".

**T2 — ¿el cuaderno ayuda a JEV a reconocer el siguiente eslabón?** AUC dentro de cada objetivo (oro vs. 4 no-oro del mismo pozo), pregunta `next_needed`:

| estado | intermedios (63) | finales (150) |
|---|---|---|
| orden del pozo de búsqueda (referencia) | 0.817 | 0.933 |
| sin cuaderno (hoy) | 0.786 | 0.897 |
| hechos oráculo + frontera | **0.901** (+0.115, IC +0.050..+0.177) | **0.997** (+0.100) |
| hechos oráculo solos | 0.893 (+0.107) | 0.988 |
| hechos equivocados (otra pregunta) | 0.752 (−0.034, IC −0.088..+0.019) | 0.898 (+0.002) |
| **hechos escogidos por JEV (T3)** | **0.871** (+0.085, IC +0.022..+0.150) | **0.974** (+0.077) |

**Puerta P1 — se cumple:** el oráculo sube la AUC de intermedios +0.115 (≥ 0.10; el límite inferior del IC es +0.05) y los hechos equivocados la bajan −0.034 (≤ 0.05). Con hechos escogidos por el propio JEV se conserva ~75% de la ganancia.

**Lectura:** el cuaderno ataca justo la falla de §21: (1) hace que la búsqueda alcance los puentes intermedios, (2) JEV escoge bien la frase clave (muy por encima de heurísticas) y (3) con esa frase en el estado JEV ordena mejor que sin ella y que el propio orden de búsqueda. Sin cuaderno, JEV queda por debajo del orden de búsqueda (0.786 vs 0.817), coherente con §21. Falta: la frontera (entidad) aquí es la respuesta oro; en el bucle real sale de la frase escogida (JEV no escribe), así que la búsqueda de la ronda 2 debe usar la frase completa (vector) + BM25 de la frase. Siguiente: Fase 2 en pequeño (bucle real de 2–3 rondas vs. una pasada con el mismo presupuesto) en un dev nuevo, y reordenador estándar (Fase 1) en cuanto se habilite huggingface.co.

---

## 24. Fase 2 — bucle con cuaderno: resultados en dev y pre-registro (3 de octubre de 2026)

*Escrito después de dev y antes de construir la memoria nueva.*

**Dev (34 preguntas con respuesta de la memoria MuSiQue de §20; `results/20261003-050717-musique-explore-dev.md`):**

| sistema | R@10 | cadena@10 | encontrado (cualquier posición) | llamadas JEV / pregunta |
|---|---|---|---|---|
| vector con la pregunta (opción 1) | 0.819 | 0.588 | 0.706 | 0 |
| bucle con cuaderno sin JEV (reglas de palabras) | 0.777 | 0.500 | 0.794 | 0 |
| una pasada + JEV califica 30 | 0.806 | 0.559 | 0.706 | 30 |
| una pasada + JEV califica 60 (mismo presupuesto) | 0.794 | 0.559 | 0.853 | 60 |
| **bucle con cuaderno + JEV** | **0.926** | **0.794** | **0.882** | 66 |

Diferencia pareada en cadena@10, bucle + JEV − una pasada con mismo presupuesto: **+0.235 (IC95 +0.088..+0.382)**; − opción 1: +0.206; − bucle sin JEV: +0.294. Por pasos: 3 saltos 0.80 vs 0.40–0.53.

Ajuste hecho en dev (en 10 preguntas): la ronda 1 busca solo por vector (la mezcla con palabras clave sobre la pregunta larga hundía la búsqueda); las rondas siguientes mezclan vector (pregunta + cuaderno) con palabras clave sobre los hechos nuevos.

**Pre-registro H6.** En una memoria nueva (MuSiQue train, 20 páginas de 100 filas repartidas, 150 preguntas con respuesta + 150 sin respuesta por hash, misma regla anti-fuga de §20; nunca usada para ajustar), sobre todas las preguntas con respuesta: cadena@10 del bucle + JEV − una pasada + JEV con 60 pasajes ≥ +0.05 y el IC95 pareado (remuestreo por pregunta) excluye 0. Secundarias, sin corrección: contra opción 1 y contra el bucle sin JEV; por número de saltos.

**Configuración congelada:** `--rounds 3 --per-round 10 --read-top 3 --fact-threshold 0.5`, ≤ 2 hechos por pasaje, cuaderno ≤ 8 hechos, `--control-n 60`, JEV `jev-1.13.0`, preguntas JEV de `internal/notebook/judge.go` (las de P1), embeddings gemini-embedding-001 3072. Una sola corrida: `--dataset musique --musique-split train --mode all --explore`.

**Desviación registrada antes de correr:** la muestra de train dio una memoria de 3,402 párrafos con 150 preguntas con respuesta y solo 39 sin respuesta que pasan la regla anti-fuga (en train los párrafos se repiten mucho entre preguntas cercanas). H6 usa solo las 150 con respuesta, así que no cambia; la memoria es más chica que la de §20 (menos distractores).

---

## 25. Resultado H6 — Fase 2 en memoria nueva (3 de octubre de 2026)

*Una sola corrida con la configuración congelada de §24. Memoria nueva de MuSiQue train: 3,402 párrafos, 150 preguntas con respuesta (112 de 2 saltos, 26 de 3, 12 de 4). `results/20261003-052414-musique-train-explore-all.md`. Costo: Gemini ~US$0.065, JEV ~US$0.57 (las tres variantes con JEV).*

| sistema | R@10 | cadena@5 | cadena@10 | llamadas JEV / pregunta |
|---|---|---|---|---|
| vector con la pregunta (opción 1) | 0.876 | 0.640 | 0.727 | 0 |
| bucle con cuaderno sin JEV | 0.826 | 0.567 | 0.673 | 0 |
| una pasada + JEV califica 30 | 0.890 | 0.633 | 0.767 | 30 |
| una pasada + JEV califica 60 (mismo presupuesto) | 0.883 | 0.620 | 0.753 | 60 |
| **bucle con cuaderno + JEV** | **0.926** | **0.700** | **0.827** | 66 |

**H6 — se cumple.** Cadena@10, bucle + JEV − una pasada con 60: **+0.073 (IC95 +0.033..+0.120)**: ≥ +0.05 y el IC excluye 0. Secundarias: − opción 1 +0.100 (+0.047..+0.160); − bucle sin JEV +0.153 (+0.087..+0.227); − una pasada con 30 +0.060 (+0.020..+0.107). Por saltos (cadena@10): 2 → 0.95 vs 0.88; 3 → 0.46 vs 0.38; 4 → 0.50 vs 0.33.

**Lectura:** la ganancia se sostiene en preguntas nuevas, más chica que en dev (+0.235 en 34 preguntas; esperable por el tamaño de dev y porque aquí 75% son de 2 saltos). El cuaderno sin JEV empeora a la búsqueda simple: lo que hace funcionar el bucle es que JEV escoja las frases y juzgue con ellas. JEV que reordena sin descartar (una pasada) ya no daña (+0.03–0.04 sobre opción 1), a diferencia de la opción 2 que descartaba. Pendientes: reordenador estándar (Fase 1), abstención por cobertura (Fase 3), y dominios de empresa / español (Fase 4).

---

## 26. Fase 3 — abstención: dev y pre-registro (3 de octubre de 2026)

*Escrito después de dev (memoria MuSiQue de §20, 34 con respuesta + 48 sin respuesta; `results/*-musique-abstain-dev.md`) y antes de tocar la memoria de prueba.*

**Dev (AUROC, mayor = separa mejor contestables de no contestables):** filtro anterior (JEV sobre los 5 primeros del vector) 0.771; cuaderno `answer_stated` 0.765; cuaderno `1 − missing_link` 0.767; cobertura (media de ambas) 0.767; máximo `answers_query` del bucle 0.690; cuaderno + 5 mejores pasajes del bucle 0.738. **Media(filtro anterior, cobertura del cuaderno) 0.807** (mejor exactitud balanceada 0.765 con umbral 0.31).

Diagnóstico: en las preguntas con respuesta cuya cadena el bucle sí trajo completa (27), la cobertura separa a 0.816; en las 7 con cadena incompleta la cobertura es baja (0.15), que es lo correcto (no hay evidencia suficiente en lo recuperado). Ninguna señal sola llega a 0.85 en dev.

**Pre-registro H7.** Memoria nueva MuSiQue train con `--musique-salt b --musique-pages 80` (5,161 párrafos; 150 con respuesta + 150 sin respuesta; nunca usada). Señal primaria: media(filtro anterior, cobertura del cuaderno); umbral congelado 0.31. Meta del plan: AUROC ≥ 0.85 y exactitud balanceada ≥ 0.80. Secundarias: AUROC de cada señal y del filtro anterior solo, con IC95 por remuestreo de preguntas. Una sola corrida: `--dataset musique --musique-split train --musique-salt b --musique-pages 80 --mode all --abstain --abstain-threshold 0.31`. La misma corrida da cadena@10 del bucle en esa memoria (réplica de H6 en otra muestra).

---

## 27. Resultados H7 (abstención) y Fase 1 en dev; pre-registro H8 (3 de octubre de 2026)

**H7 — no se cumple (por poco).** Memoria B (MuSiQue train, sal "b"): 150 con respuesta + 150 sin respuesta, 5,161 párrafos. Señal primaria media(filtro anterior, cobertura del cuaderno): **AUROC 0.849 (IC95 0.802–0.888)**, exactitud balanceada con umbral congelado 0.31: **0.780** (metas 0.85 y 0.80). Secundarias: filtro anterior solo 0.830; cobertura del cuaderno 0.818; exploratorias no pre-registradas: cuaderno + 5 mejores pasajes del bucle 0.860, media(filtro sobre top-5 del bucle, cuaderno + pasajes) 0.861 (mejor exactitud balanceada 0.807). Recuperación del bucle en esa memoria: R@10 0.914, cadena@10 0.813 (réplica de H6 en otra muestra). `results/20261003-070134-musique-trainb-abstain-all.md`. Costo JEV ~US$0.71.

**Fase 1 en dev** (`results/20261003-065445-musique-explore-phase1-dev.md`, bge-reranker-v2-m3 int8 en CPU; Spearman 0.991 contra fp32): cadena@10 — vector 0.588; una pasada + JEV 30 0.559; **una pasada + reordenador 30 0.441**; reordenador 60 0.265; media JEV+reordenador 30 0.588; bucle con reordenador (sin JEV) 0.324; bucle reordenador califica + JEV escoge frases 0.294; bucle media + JEV frases 0.559; **bucle + JEV 0.794**. El reordenador, como JEV aislado en §21, hunde los puentes (le da 0.02 al pasaje intermedio del ejemplo Coolidge) y además rinde por debajo del embedding de Gemini como ordenador.

**Pre-registro H8 (memoria A de §25, 150 con respuesta, una corrida `--variants phase1final --musique-split train --mode all`):** (a) cadena@10 bucle + JEV − una pasada + reordenador 30 > 0 con IC95 que excluye 0; (b) puerta de la Fase 1: si una pasada + reordenador 30 iguala o supera a una pasada + JEV 30 (±0.01), JEV deja de ordenar en una pasada.
