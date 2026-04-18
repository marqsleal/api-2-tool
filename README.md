# 🚀 API-2-Tool

## Visão geral

O **API-2-Tool** é uma prova de conceito que transforma APIs HTTP em ferramentas compatíveis com **tool calling para agentes LLM**.

A aplicação funciona como um middleware:

* recebe o contrato de uma API
* expõe esse contrato como uma tool
* executa chamadas upstream
* retorna a resposta em formato padronizado (`function_call_output`)

---

## Problema

Em ambientes com múltiplas APIs:

* contratos são heterogêneos
* integrações são específicas por domínio
* agentes precisam conhecer cada API individualmente

Isso gera:

* alto acoplamento
* duplicação de lógica
* custo de manutenção elevado

---

## Solução

O projeto introduz uma camada intermediária que:

* abstrai APIs como tools
* padroniza execução
* desacopla agentes do domínio das APIs

O agente passa a consumir apenas tools, não APIs diretamente.

---

## Como funciona

Fluxo:

```text
Agent
  ↓
API-2-Tool
  ↓
API externa
  ↓
Resposta normalizada (function_call_output)
```

O padrão de interação utilizado é o [Function Calling](https://developers.openai.com/api/docs/guides/function-calling).

Nesse modelo, a LLM não executa chamadas externas diretamente. Ela retorna uma intenção estruturada (tool call), indicando qual função deve ser executada e com quais argumentos. A execução da chamada é responsabilidade do sistema, neste caso, o API-2-Tool.

---

## Funcionalidades

* `POST /tool` — cadastrar tool
* `GET /tool/definitions` — listar tools
* `GET /tool/definitions/{id}` — obter tool
* `PATCH /tool/definitions/{id}` — atualizar tool
* `DELETE /tool/definitions/{id}` — desativar tool
* `POST /tool/execute/{id}` — executar tool
* `GET /health` — health check

A especificação OpenAPI está disponível via Swagger.

---

## Arquitetura

Organização baseada em camadas:

* `handler` → HTTP
* `service` → regras de negócio
* `repository` → persistência
* `domain` → modelos
* `router` → composição

Padrão:

```text
Handler → Service → Repository
```

Persistência via SQLite.

---

## Tecnologias

* Go (`net/http`)
* SQLite
* OpenAPI 3
* Swagger UI
* Docker / Docker Compose

---

## Exemplo

Cadastro:

```json
{
  "name": "viacep_consultar_cep",
  "description": "Consulta dados de endereco no ViaCEP a partir de um CEP brasileiro. Retorna logradouro, bairro, cidade, estado, IBGE, DDD e metadados oficiais para enriquecer respostas de agentes.",
  "method": "GET",
  "url": "https://viacep.com.br/ws/{cep}/json/",
  "headers": {},
  "parameters": {
    "type": "object",
    "additionalProperties": false,
    "required": ["cep"],
    "properties": {
      "cep": {
        "type": "string",
        "description": "CEP brasileiro com 8 digitos numericos (somente numeros). Exemplo: 01001000.",
        "pattern": "^[0-9]{8}$",
        "examples": ["01001000", "30140071"]
      },
      "include_geodata": {
        "type": "boolean",
        "description": "Quando true, manter campos de codigos oficiais (ibge, gia, siafi) na resposta resumida do agente.",
        "default": true
      }
    }
  },
  "strict": true
}
```

Execução:

```json
{
  "call_id": "call_123",
  "arguments": {
    "cep": "01001000"
  }
}
```

Resposta:

```json
{
  "type": "function_call_output",
  "call_id": "call_123",
  "output": "{ ... }"
}
```

---

## Pasta `chat/`

A pasta `chat/` contém uma aplicação auxiliar em Python usada para **validar o uso da API-2-Tool com agentes**.

Ela:

* simula um agente
* consome tools registradas
* executa chamadas via API-2-Tool

Não faz parte do core do sistema.

Serve apenas para:

* validar o conceito
* testar integrações
* demonstrar o fluxo agent → tool → API

---

## Limitações

Este projeto é uma PoC.

Não inclui:

* autenticação/autorização
* rate limiting
* gestão de secrets
* observabilidade avançada

---

## Uso responsável

O uso deve respeitar os contratos das APIs integradas:

* termos de uso
* limites de requisição
* políticas de autenticação

**Ética é inegociável.**

---

## Como rodar

### Local

```bash
cd app
go mod tidy
go run cmd/api/main.go
```

API disponível em:

```
http://localhost:8080
```

---

### Docker

```bash
docker-compose up --build
```

---

### Swagger

```
http://localhost:8080/swagger/index.html
```

---

### Health check

```bash
curl http://localhost:8080/health
```

---

## Estrutura

```text
app/
  cmd/api/
  internal/
    handler/
    service/
    repository/
    domain/
    router/

chat/        → validação com agente
scripts/     → exemplos de uso
```

---

## Evoluções possíveis

* autenticação por tool
* rate limiting
* caching
* versionamento de contratos

---

## Licença

Ver `LICENSE`.

> Este projeto é uma prova de conceito independente, desenvolvido para fins de estudo.
> Não possui qualquer relação com sistemas, dados ou arquitetura de instituições públicas ou privadas.
