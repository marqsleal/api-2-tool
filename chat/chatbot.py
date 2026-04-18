#!/usr/bin/env python3
import json
import logging
import os
import re
import time
import uuid
from typing import Any, Dict, List, Literal, Optional

import requests
from fastapi import APIRouter, FastAPI, HTTPException
from pydantic import BaseModel, Field


LOG_LEVEL = os.getenv("CHATBOT_LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
    datefmt="%Y/%m/%d %H:%M:%S",
)
logger = logging.getLogger(__name__)


def load_env_file(path: str) -> None:
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as file:
        for raw in file:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            key = key.strip()
            value = value.strip()
            if key and key not in os.environ:
                os.environ[key] = value


load_env_file(os.path.join(os.path.dirname(__file__), ".env"))

OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://localhost:11434").rstrip("/")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "phi3:mini")
TOOLS_API_BASE_URL = os.getenv("TOOLS_API_BASE_URL", "http://localhost:8080").rstrip("/")
TOOLS_CONNECT_TIMEOUT_SECONDS = int(os.getenv("TOOLS_CONNECT_TIMEOUT_SECONDS", "5"))
TOOLS_READ_TIMEOUT_SECONDS = int(os.getenv("TOOLS_READ_TIMEOUT_SECONDS", "30"))
OLLAMA_CONNECT_TIMEOUT_SECONDS = int(os.getenv("OLLAMA_CONNECT_TIMEOUT_SECONDS", "5"))
OLLAMA_READ_TIMEOUT_SECONDS = int(os.getenv("OLLAMA_READ_TIMEOUT_SECONDS", "180"))
MAX_TOOL_STEPS = int(os.getenv("MAX_TOOL_STEPS", "5"))

SYSTEM_PROMPT_TEMPLATE = """
Voce e um assistente de tarefas que resolve perguntas usando a API de Tools.
Seu trabalho e:
1) entender a pergunta do usuario;
2) decidir quais tools chamar e com quais argumentos;
3) iterar tool(s) ate ter dados suficientes;
4) entregar resposta final clara e correta.

Durante a etapa de planejamento/execucao, responda APENAS em JSON valido (sem markdown):

1) resposta final:
{{"action":"answer","answer":"texto"}}

2) chamada de tool:
{{"action":"call_tool","tool_name":"nome_exato","arguments":{{}}}}

Regras:
- use tool_name exatamente igual ao catalogo
- se ja existir resultado da tool na conversa, responda com action=answer
- se faltar dado para responder com confianca, chame tool ao inves de inventar
- prefira respostas objetivas e rastreaveis aos resultados das tools

Exemplo de pergunta complexa:
"Hoje e dia 18 de abril de 2026, qual o proximo feriado no Brasil?"
Fluxo esperado:
- chamar tool de feriados com ano/pais adequados;
- analisar datas retornadas;
- responder com o proximo feriado em relacao a 18/04/2026.

Catalogo de tools:
{catalog}
""".strip()

TOOL_RESULT_PROMPT_TEMPLATE = """
Resultado da tool executada:
{tool_result}

Com base nisso, decida o proximo passo:
- Se precisar de mais dados, responda com:
{{"action":"call_tool","tool_name":"nome_exato","arguments":{{...}}}}
- Se ja puder concluir, responda com:
{{"action":"answer","answer":"texto final"}}
""".strip()

FORCE_FINAL_ANSWER_PROMPT = """
Voce atingiu o limite de chamadas de tools nesta requisicao.
Agora responda ao usuario com a melhor resposta final possivel, sem chamar novas tools.
Formato obrigatorio:
{{"action":"answer","answer":"texto final"}}
""".strip()

INVALID_DECISION_PROMPT = """
Sua resposta anterior nao estava em um JSON de decisao valido.
Responda APENAS em JSON com um destes formatos:
{{"action":"call_tool","tool_name":"nome_exato","arguments":{{...}}}}
ou
{{"action":"answer","answer":"texto final"}}
""".strip()

FINAL_RESPONSE_SYSTEM_PROMPT = """
Voce e um assistente finalizador de respostas.
Recebera:
- a pergunta original do usuario;
- um rascunho de resposta;
- os resultados brutos de tools executadas.

Sua tarefa:
- produzir a MELHOR resposta final em linguagem natural (na lingua do usuario);
- responder diretamente ao que foi perguntado;
- usar os resultados das tools como base factual;
- quando houver datas, mencionar data explicita no formato DD/MM/AAAA;
- nao retornar JSON nem markdown, apenas texto final.
""".strip()

FINAL_RESPONSE_USER_PROMPT_TEMPLATE = """
Pergunta original:
{question}

Rascunho atual:
{draft}

Resultados de tools (evidencias):
{tool_evidence}
""".strip()


class Message(BaseModel):
    role: Literal["user", "assistant", "system"]
    content: str


class ChatRequest(BaseModel):
    message: str = Field(min_length=1)
    history: List[Message] = Field(default_factory=list)


class ChatResponse(BaseModel):
    answer: str
    tool_used: Optional[str] = None
    tool_output: Optional[Any] = None


class ToolDecision(BaseModel):
    action: Literal["answer", "call_tool"]
    answer: Optional[str] = None
    tool_name: Optional[str] = None
    arguments: Dict[str, Any] = Field(default_factory=dict)


def normalize_user_message(message: str) -> str:
    text = message.strip()
    # Collapse repeated whitespace to reduce prompt noise while preserving meaning.
    text = re.sub(r"\s+", " ", text)
    return text


def normalize_tool_arguments(arguments: Dict[str, Any], aggressive: bool = False) -> Dict[str, Any]:
    normalized: Dict[str, Any] = {}
    for key, value in arguments.items():
        normalized_key = str(key).strip()
        normalized_value = value

        if isinstance(value, str):
            normalized_value = value.strip()
            key_lower = normalized_key.lower()

            # Generic normalization for postal-like fields.
            if any(token in key_lower for token in ["cep", "postal", "zipcode", "zip_code", "zip"]):
                digits = re.sub(r"\D", "", normalized_value)
                if digits:
                    normalized_value = digits
            elif aggressive:
                digits = re.sub(r"\D", "", normalized_value)
                if len(digits) >= 6:
                    normalized_value = digits

            if key_lower in {"country", "country_code", "pais", "codigo_pais"}:
                normalized_value = normalized_value.upper()

        normalized[normalized_key] = normalized_value
    return normalized


def parse_llm_arguments(raw_arguments: Any) -> Dict[str, Any]:
    if isinstance(raw_arguments, dict):
        return raw_arguments

    if isinstance(raw_arguments, str):
        parsed_json = extract_embedded_json(raw_arguments)
        if isinstance(parsed_json, dict):
            return parsed_json
    return {}


def output_indicates_invalid_input(tool_output: Any) -> bool:
    if not isinstance(tool_output, dict):
        return False

    status_code = tool_output.get("status_code")
    if isinstance(status_code, int) and status_code >= 400:
        return True

    response = tool_output.get("response")
    if isinstance(response, dict):
        if response.get("erro") is True:
            return True
        text = json.dumps(response, ensure_ascii=False).lower()
    else:
        text = str(response).lower()

    invalid_markers = ["invalid", "invalido", "inválido", "malformed", "erro"]
    return any(marker in text for marker in invalid_markers)


def to_raw_output_text(value: Any) -> str:
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, indent=2)
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False)


def unwrap_answer_text(raw_text: str) -> str:
    text = raw_text.strip()
    if not text:
        return ""

    try:
        parsed = ToolDecision.model_validate_json(text)
        if parsed.action == "answer":
            logger.info("unwrap answer parsed_from=tool_decision")
            return (parsed.answer or "").strip()
    except Exception:
        pass

    try:
        generic = json.loads(text)
        if isinstance(generic, dict) and "answer" in generic:
            logger.info("unwrap answer parsed_from=generic_json")
            return str(generic.get("answer", "")).strip()
    except Exception:
        pass

    logger.info("unwrap answer parsed_from=raw_text")
    return text


def extract_embedded_json(text: str) -> Optional[Any]:
    content = text.strip()
    if not content:
        return None

    try:
        return json.loads(content)
    except Exception:
        pass

    code_block = re.search(r"```(?:json)?\s*([\[{].*[\]}])\s*```", content, flags=re.DOTALL)
    if code_block:
        candidate = code_block.group(1).strip()
        try:
            return json.loads(candidate)
        except Exception:
            pass

    for idx in range(len(content)):
        if content[idx] not in "{[":
            continue
        candidate = content[idx:].strip()
        try:
            return json.loads(candidate)
        except Exception:
            continue

    return None


def extract_json_objects(text: str) -> List[Dict[str, Any]]:
    decoder = json.JSONDecoder()
    content = text.strip()
    objects: List[Dict[str, Any]] = []
    idx = 0
    while idx < len(content):
        ch = content[idx]
        if ch not in "{[":
            idx += 1
            continue
        try:
            value, end = decoder.raw_decode(content[idx:])
            if isinstance(value, dict):
                objects.append(value)
            idx += max(end, 1)
        except Exception:
            idx += 1
    return objects


def coerce_tool_decision(candidate: Dict[str, Any]) -> Optional[ToolDecision]:
    action = candidate.get("action")
    if action not in {"answer", "call_tool"}:
        return None

    if action == "answer":
        answer_value = candidate.get("answer", "")
        if isinstance(answer_value, str):
            answer_text = answer_value
        else:
            answer_text = json.dumps(answer_value, ensure_ascii=False)
        return ToolDecision(action="answer", answer=answer_text, tool_name=None, arguments={})

    tool_name = str(candidate.get("tool_name", "")).strip()
    arguments = parse_llm_arguments(candidate.get("arguments", {}))
    if not tool_name:
        return None
    return ToolDecision(action="call_tool", tool_name=tool_name, arguments=arguments, answer=None)


def parse_tool_decision(raw_text: str) -> Optional[ToolDecision]:
    text = raw_text.strip()
    if not text:
        return None

    try:
        return ToolDecision.model_validate_json(text)
    except Exception:
        pass

    extracted = extract_embedded_json(text)
    if isinstance(extracted, dict):
        try:
            return ToolDecision.model_validate(extracted)
        except Exception:
            coerced = coerce_tool_decision(extracted)
            if coerced:
                return coerced

    candidates = extract_json_objects(text)
    if not candidates:
        return None

    decisions: List[ToolDecision] = []
    for candidate in candidates:
        try:
            decisions.append(ToolDecision.model_validate(candidate))
            continue
        except Exception:
            pass
        coerced = coerce_tool_decision(candidate)
        if coerced:
            decisions.append(coerced)

    if not decisions:
        return None

    for decision in decisions:
        if decision.action == "call_tool":
            return decision
    for decision in decisions:
        if decision.action == "answer":
            return decision
    return None


def synthesize_final_answer(
    question: str,
    draft_answer: str,
    tool_events: List[Dict[str, Any]],
) -> str:
    evidence = json.dumps(tool_events, ensure_ascii=False)
    user_prompt = FINAL_RESPONSE_USER_PROMPT_TEMPLATE.format(
        question=question,
        draft=draft_answer,
        tool_evidence=evidence,
    )
    payload = {
        "model": OLLAMA_MODEL,
        "stream": False,
        "messages": [
            {"role": "system", "content": FINAL_RESPONSE_SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        "options": {"temperature": 0.1},
    }
    logger.info("final synthesis request started evidence_items=%d", len(tool_events))
    response = requests.post(
        f"{OLLAMA_BASE_URL}/api/chat",
        json=payload,
        timeout=(OLLAMA_CONNECT_TIMEOUT_SECONDS, OLLAMA_READ_TIMEOUT_SECONDS),
    )
    response.raise_for_status()
    final_text = str(response.json().get("message", {}).get("content", "")).strip()
    logger.info("final synthesis request finished content_len=%d", len(final_text))
    return final_text


def list_tool_definitions() -> List[Dict[str, Any]]:
    logger.info("tools definitions request started url=%s/tool/definitions", TOOLS_API_BASE_URL)
    response = requests.get(
        f"{TOOLS_API_BASE_URL}/tool/definitions",
        timeout=(TOOLS_CONNECT_TIMEOUT_SECONDS, TOOLS_READ_TIMEOUT_SECONDS),
    )
    response.raise_for_status()
    items = response.json().get("items", [])
    logger.info("tools definitions request finished count=%d", len(items))
    return items


def format_tool_catalog(definitions: List[Dict[str, Any]]) -> str:
    lines: List[str] = []
    for definition in definitions:
        function_data = definition.get("tool", {}).get("function", {})
        lines.append(
            json.dumps(
                {
                    "id": definition.get("id"),
                    "name": function_data.get("name"),
                    "description": function_data.get("description", ""),
                    "parameters": function_data.get("parameters", {}),
                },
                ensure_ascii=False,
            )
        )
    return "\n".join(lines) if lines else "(sem tools cadastradas)"


def dedupe_definitions_by_name(definitions: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    unique: List[Dict[str, Any]] = []
    indexes: Dict[str, int] = {}
    for definition in definitions:
        name = str(definition.get("tool", {}).get("function", {}).get("name", "")).strip().lower()
        if not name:
            continue
        if name in indexes:
            unique[indexes[name]] = definition
        else:
            indexes[name] = len(unique)
            unique.append(definition)
    if len(unique) != len(definitions):
        logger.info("tools definitions deduped before=%d after=%d", len(definitions), len(unique))
    return unique


def call_ollama(messages: List[Dict[str, str]], system_prompt: str) -> str:
    payload = {
        "model": OLLAMA_MODEL,
        "stream": False,
        "messages": [{"role": "system", "content": system_prompt}] + messages,
        "options": {"temperature": 0.1},
    }
    logger.info(
        "ollama chat request started model=%s message_count=%d connect_timeout=%d read_timeout=%d",
        OLLAMA_MODEL,
        len(payload["messages"]),
        OLLAMA_CONNECT_TIMEOUT_SECONDS,
        OLLAMA_READ_TIMEOUT_SECONDS,
    )
    response = requests.post(
        f"{OLLAMA_BASE_URL}/api/chat",
        json=payload,
        timeout=(OLLAMA_CONNECT_TIMEOUT_SECONDS, OLLAMA_READ_TIMEOUT_SECONDS),
    )
    response.raise_for_status()
    content = str(response.json().get("message", {}).get("content", "")).strip()
    logger.info("ollama chat request finished content_len=%d", len(content))
    return content


def find_tool_id_by_name(definitions: List[Dict[str, Any]], tool_name: str) -> Optional[str]:
    expected = tool_name.lower().strip()
    for definition in definitions:
        name = str(definition.get("tool", {}).get("function", {}).get("name", "")).lower().strip()
        if name == expected:
            return str(definition.get("id", ""))
    return None


def execute_tool(tool_id: str, arguments: Dict[str, Any]) -> Any:
    call_id = f"call_{uuid.uuid4()}"
    payload = {"call_id": call_id, "arguments": arguments}
    logger.info(
        "tool execute request started tool_id=%s call_id=%s argument_keys=%s",
        tool_id,
        call_id,
        list(arguments.keys()),
    )
    response = requests.post(
        f"{TOOLS_API_BASE_URL}/tool/execute/{tool_id}",
        json=payload,
        timeout=(TOOLS_CONNECT_TIMEOUT_SECONDS, TOOLS_READ_TIMEOUT_SECONDS),
    )
    response.raise_for_status()
    raw = response.json().get("output")
    if isinstance(raw, str):
        try:
            parsed = json.loads(raw)
            logger.info("tool execute request finished tool_id=%s output_type=json", tool_id)
            return parsed
        except json.JSONDecodeError:
            logger.info("tool execute request finished tool_id=%s output_type=string", tool_id)
            return raw
    logger.info("tool execute request finished tool_id=%s output_type=%s", tool_id, type(raw).__name__)
    return raw


def register_routes(router: APIRouter) -> None:
    @router.get("/health")
    def health() -> Dict[str, str]:
        return {"status": "ok"}

    @router.post("/chat", response_model=ChatResponse)
    def chat(request: ChatRequest) -> ChatResponse:
        request_id = f"chat_{uuid.uuid4().hex[:8]}"
        started_at = time.monotonic()
        logger.info(
            "chat request started request_id=%s history_items=%d message_len=%d",
            request_id,
            len(request.history),
            len(request.message),
        )
        normalized_user_message = normalize_user_message(request.message)
        if normalized_user_message != request.message:
            logger.info(
                "chat user message normalized request_id=%s before=%r after=%r",
                request_id,
                request.message,
                normalized_user_message,
            )

        try:
            definitions = dedupe_definitions_by_name(list_tool_definitions())
        except requests.RequestException as exc:
            logger.error("chat failed request_id=%s stage=definitions err=%s", request_id, exc)
            raise HTTPException(status_code=502, detail="failed to load tool definitions") from exc

        system_prompt = SYSTEM_PROMPT_TEMPLATE.format(catalog=format_tool_catalog(definitions))
        messages = [m.model_dump() for m in request.history]
        messages.append({"role": "user", "content": normalized_user_message})
        logger.info(
            "chat request context request_id=%s tools_count=%d prompt_chars=%d max_tool_steps=%d",
            request_id,
            len(definitions),
            len(system_prompt),
            MAX_TOOL_STEPS,
        )

        last_tool_name: Optional[str] = None
        last_tool_output: Optional[Any] = None
        tool_events: List[Dict[str, Any]] = []

        for step in range(1, MAX_TOOL_STEPS + 1):
            try:
                reply = call_ollama(messages, system_prompt)
            except requests.Timeout as exc:
                logger.error("chat failed request_id=%s stage=ollama_step_%d timeout err=%s", request_id, step, exc)
                raise HTTPException(status_code=504, detail="ollama timeout") from exc
            except requests.RequestException as exc:
                logger.error("chat failed request_id=%s stage=ollama_step_%d err=%s", request_id, step, exc)
                raise HTTPException(status_code=502, detail="ollama unavailable") from exc

            decision = parse_tool_decision(reply)
            if decision is None:
                logger.warning("chat decision invalid request_id=%s step=%d", request_id, step)
                messages.append({"role": "assistant", "content": reply})
                messages.append({"role": "user", "content": INVALID_DECISION_PROMPT})
                continue

            if decision.action == "answer":
                logger.info("chat decision request_id=%s step=%d mode=answer_json", request_id, step)
                draft = (decision.answer or "").strip() or unwrap_answer_text(reply)
                final_answer = draft
                if tool_events:
                    try:
                        final_answer = synthesize_final_answer(normalized_user_message, draft, tool_events)
                    except requests.Timeout as exc:
                        logger.error("chat final synthesis timeout request_id=%s err=%s", request_id, exc)
                    except requests.RequestException as exc:
                        logger.error("chat final synthesis error request_id=%s err=%s", request_id, exc)
                duration_ms = int((time.monotonic() - started_at) * 1000)
                logger.info(
                    "chat request finished request_id=%s tool_used=%s duration_ms=%d",
                    request_id,
                    bool(last_tool_name),
                    duration_ms,
                )
                return ChatResponse(
                    answer=final_answer,
                    tool_used=last_tool_name,
                    tool_output=last_tool_output,
                )

            tool_name = (decision.tool_name or "").strip()
            raw_arguments = parse_llm_arguments(decision.arguments)
            arguments = normalize_tool_arguments(raw_arguments)
            logger.info(
                "chat decision request_id=%s step=%d mode=call_tool tool_name=%s argument_keys=%s",
                request_id,
                step,
                tool_name,
                list(arguments.keys()),
            )
            if arguments != raw_arguments:
                logger.info(
                    "chat tool arguments normalized request_id=%s step=%d tool_name=%s before=%s after=%s",
                    request_id,
                    step,
                    tool_name,
                    raw_arguments,
                    arguments,
                )

            tool_id = find_tool_id_by_name(definitions, tool_name)
            if not tool_id:
                logger.warning(
                    "chat invalid tool request_id=%s step=%d unknown_tool=%s",
                    request_id,
                    step,
                    tool_name,
                )
                messages.append({"role": "assistant", "content": reply})
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            f"A tool '{tool_name}' nao existe no catalogo atual. "
                            "Use um nome exato de tool valida e responda em JSON."
                        ),
                    }
                )
                continue

            try:
                tool_output = execute_tool(tool_id, arguments)
            except requests.RequestException as exc:
                logger.error(
                    "chat failed request_id=%s stage=execute_tool step=%d tool_id=%s err=%s",
                    request_id,
                    step,
                    tool_id,
                    exc,
                )
                messages.append({"role": "assistant", "content": reply})
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            f"Falha ao executar tool '{tool_name}' (id={tool_id}): {exc}. "
                            "Ajuste os argumentos ou escolha outra tool, respondendo em JSON."
                        ),
                    }
                )
                continue

            # One retry path for likely input-format errors (e.g., formatted postal codes).
            if output_indicates_invalid_input(tool_output):
                retried_arguments = normalize_tool_arguments(arguments, aggressive=True)
                if retried_arguments != arguments:
                    logger.info(
                        "chat tool retry normalized request_id=%s step=%d tool_name=%s before=%s after=%s",
                        request_id,
                        step,
                        tool_name,
                        arguments,
                        retried_arguments,
                    )
                    try:
                        tool_output = execute_tool(tool_id, retried_arguments)
                        arguments = retried_arguments
                    except requests.RequestException as exc:
                        logger.error(
                            "chat failed request_id=%s stage=execute_tool_retry step=%d tool_id=%s err=%s",
                            request_id,
                            step,
                            tool_id,
                            exc,
                        )

            last_tool_name = tool_name
            last_tool_output = tool_output
            tool_events.append(
                {
                    "tool_name": tool_name,
                    "tool_id": tool_id,
                    "arguments": arguments,
                    "output": tool_output,
                }
            )

            tool_result = json.dumps(
                {
                    "tool_name": tool_name,
                    "tool_id": tool_id,
                    "arguments": arguments,
                    "output": tool_output,
                },
                ensure_ascii=False,
            )
            followup_prompt = TOOL_RESULT_PROMPT_TEMPLATE.format(tool_result=tool_result)
            messages.append({"role": "assistant", "content": reply})
            messages.append({"role": "user", "content": followup_prompt})
            logger.info(
                "chat loop continue request_id=%s step=%d tool_name=%s followup_chars=%d",
                request_id,
                step,
                tool_name,
                len(followup_prompt),
            )

        logger.warning("chat max steps reached request_id=%s max_tool_steps=%d", request_id, MAX_TOOL_STEPS)
        messages.append({"role": "user", "content": FORCE_FINAL_ANSWER_PROMPT})
        try:
            forced_reply = call_ollama(messages, system_prompt)
        except requests.Timeout as exc:
            logger.error("chat failed request_id=%s stage=force_final timeout err=%s", request_id, exc)
            raise HTTPException(status_code=504, detail="ollama timeout") from exc
        except requests.RequestException as exc:
            logger.error("chat failed request_id=%s stage=force_final err=%s", request_id, exc)
            raise HTTPException(status_code=502, detail="ollama unavailable") from exc

        forced_decision = parse_tool_decision(forced_reply)
        final_text = (
            (forced_decision.answer or "").strip()
            if forced_decision and forced_decision.action == "answer"
            else unwrap_answer_text(forced_reply)
        )
        if tool_events:
            try:
                final_text = synthesize_final_answer(normalized_user_message, final_text, tool_events)
            except requests.Timeout as exc:
                logger.error("chat forced final synthesis timeout request_id=%s err=%s", request_id, exc)
            except requests.RequestException as exc:
                logger.error("chat forced final synthesis error request_id=%s err=%s", request_id, exc)
        duration_ms = int((time.monotonic() - started_at) * 1000)
        logger.info(
            "chat request finished request_id=%s tool_used=%s duration_ms=%d forced_final=true",
            request_id,
            bool(last_tool_name),
            duration_ms,
        )
        return ChatResponse(
            answer=final_text,
            tool_used=last_tool_name,
            tool_output=last_tool_output,
        )


def create_app() -> FastAPI:
    logger.info(
        "create app config ollama_base_url=%s ollama_model=%s tools_api_base_url=%s ollama_connect_timeout=%d ollama_read_timeout=%d tools_connect_timeout=%d tools_read_timeout=%d max_tool_steps=%d",
        OLLAMA_BASE_URL,
        OLLAMA_MODEL,
        TOOLS_API_BASE_URL,
        OLLAMA_CONNECT_TIMEOUT_SECONDS,
        OLLAMA_READ_TIMEOUT_SECONDS,
        TOOLS_CONNECT_TIMEOUT_SECONDS,
        TOOLS_READ_TIMEOUT_SECONDS,
        MAX_TOOL_STEPS,
    )
    app = FastAPI(title="POC Chatbot", version="1.0.0")
    router = APIRouter()
    register_routes(router)
    app.include_router(router)
    return app


app = create_app()
