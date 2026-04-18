import json
import logging
import os
import re
import time
import uuid
from typing import Any, Dict, List, Optional, Tuple

import requests
import streamlit as st

CHATBOT_API_URL = os.getenv("CHATBOT_API_URL", "http://localhost:8000").rstrip("/")
CHAT_ENDPOINT = f"{CHATBOT_API_URL}/chat"
CHATBOT_API_CONNECT_TIMEOUT_SECONDS = int(os.getenv("CHATBOT_API_CONNECT_TIMEOUT_SECONDS", "5"))
CHATBOT_API_READ_TIMEOUT_SECONDS = int(os.getenv("CHATBOT_API_READ_TIMEOUT_SECONDS", "600"))

LOG_LEVEL = os.getenv("CHATBOT_UI_LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
    datefmt="%Y/%m/%d %H:%M:%S",
)
logger = logging.getLogger(__name__)
logger.info(
    "ui startup chatbot_api_url=%s connect_timeout=%d read_timeout=%d",
    CHATBOT_API_URL,
    CHATBOT_API_CONNECT_TIMEOUT_SECONDS,
    CHATBOT_API_READ_TIMEOUT_SECONDS,
)

st.set_page_config(page_title="Chatbot POC", page_icon="💬", layout="centered")

st.markdown(
    """
    <style>
      :root {
        --ctp-rosewater: #f5e0dc;
        --ctp-flamingo: #f2cdcd;
        --ctp-pink: #f5c2e7;
        --ctp-mauve: #cba6f7;
        --ctp-red: #f38ba8;
        --ctp-maroon: #eba0ac;
        --ctp-peach: #fab387;
        --ctp-yellow: #f9e2af;
        --ctp-green: #a6e3a1;
        --ctp-teal: #94e2d5;
        --ctp-sky: #89dceb;
        --ctp-sapphire: #74c7ec;
        --ctp-blue: #89b4fa;
        --ctp-lavender: #b4befe;
        --ctp-text: #cdd6f4;
        --ctp-subtext1: #bac2de;
        --ctp-subtext0: #a6adc8;
        --ctp-overlay2: #9399b2;
        --ctp-overlay1: #7f849c;
        --ctp-overlay0: #6c7086;
        --ctp-surface2: #585b70;
        --ctp-surface1: #45475a;
        --ctp-surface0: #313244;
        --ctp-base: #1e1e2e;
        --ctp-mantle: #181825;
        --ctp-crust: #11111b;
      }

      .stApp {
        background: linear-gradient(180deg, var(--ctp-base) 0%, var(--ctp-mantle) 100%);
        color: var(--ctp-text);
      }

      h1, h2, h3, h4, h5, h6, p, label, span, li {
        color: var(--ctp-text) !important;
      }

      [data-testid="stSidebar"] {
        background: var(--ctp-crust);
        border-left: 1px solid var(--ctp-surface0);
      }

      [data-testid="stChatMessage"] {
        background: var(--ctp-surface0);
        border: 1px solid var(--ctp-surface1);
        border-radius: 12px;
      }

      .stTextInput input, .stChatInput input {
        background-color: var(--ctp-mantle) !important;
        color: var(--ctp-text) !important;
        border: 1px solid var(--ctp-surface1) !important;
      }

      .stButton > button {
        background: var(--ctp-blue);
        color: var(--ctp-crust);
        border: 1px solid var(--ctp-blue);
      }

      .stButton > button:hover {
        background: var(--ctp-lavender);
        border-color: var(--ctp-lavender);
      }

      code {
        background: var(--ctp-surface0) !important;
        color: var(--ctp-rosewater) !important;
      }
    </style>
    """,
    unsafe_allow_html=True,
)

st.title("Chatbot POC")
st.caption("Interface Streamlit para o endpoint /chat")

if "messages" not in st.session_state:
    st.session_state.messages = []

if "last_tool" not in st.session_state:
    st.session_state.last_tool = None


def parse_embedded_json(answer: str) -> Tuple[str, Optional[Any]]:
    text = answer.strip()
    if not text:
        return "", None

    try:
        parsed = json.loads(text)
        if isinstance(parsed, (dict, list)):
            logger.info("ui parse_embedded_json mode=full_json type=%s", type(parsed).__name__)
            return "", parsed
    except Exception:
        pass

    code_block = re.search(r"```(?:json)?\s*([\[{].*[\]}])\s*```", text, flags=re.DOTALL)
    if code_block:
        candidate = code_block.group(1).strip()
        prefix = text[: code_block.start()].strip()
        try:
            parsed = json.loads(candidate)
            if isinstance(parsed, (dict, list)):
                logger.info("ui parse_embedded_json mode=code_block type=%s", type(parsed).__name__)
                return prefix, parsed
        except Exception:
            pass

    for idx in range(len(text)):
        if text[idx] not in "{[":
            continue
        candidate = text[idx:].strip()
        try:
            parsed = json.loads(candidate)
            if isinstance(parsed, (dict, list)):
                prefix = text[:idx].strip()
                logger.info("ui parse_embedded_json mode=suffix_json type=%s", type(parsed).__name__)
                return prefix, parsed
        except Exception:
            continue

    logger.info("ui parse_embedded_json mode=none")
    return text, None


def extract_tools_from_payload(payload: Any) -> List[Dict[str, str]]:
    if not isinstance(payload, dict):
        return []

    tools: List[Dict[str, str]] = []

    if isinstance(payload.get("tools"), list):
        for item in payload["tools"]:
            if isinstance(item, dict):
                name = str(item.get("name", "")).strip()
                tool_id = str(item.get("id", "")).strip()
                if name:
                    tools.append({"name": name, "id": tool_id})

    if isinstance(payload.get("tool_names"), list):
        for name in payload["tool_names"]:
            if isinstance(name, str) and name.strip():
                tools.append({"name": name.strip(), "id": ""})

    if isinstance(payload.get("items"), list):
        for item in payload["items"]:
            if not isinstance(item, dict):
                continue
            tool_block = item.get("tool", {})
            function_block = tool_block.get("function", {}) if isinstance(tool_block, dict) else {}
            name = str(function_block.get("name", "")).strip()
            tool_id = str(item.get("id", "")).strip()
            if name:
                tools.append({"name": name, "id": tool_id})

    unique: Dict[str, Dict[str, str]] = {}
    for item in tools:
        key = item["name"].lower()
        if key not in unique:
            unique[key] = item
    return list(unique.values())


def extract_text_fields(payload: Any) -> Optional[str]:
    if not isinstance(payload, dict):
        return None

    for key in ["answer", "message", "text", "response"]:
        value = payload.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return None


def find_nested_payload(payload: Any) -> Optional[Any]:
    if not isinstance(payload, dict):
        return None

    for key in ["data", "result", "results", "output", "content", "payload"]:
        if key in payload:
            return payload[key]
    return None


def normalize_answer(answer: str) -> Tuple[str, Optional[List[Dict[str, Any]]], Optional[List[Any]]]:
    prefix, payload = parse_embedded_json(answer)
    if not payload:
        logger.info("ui normalize_answer mode=plain_text")
        return answer.strip(), None, None

    if isinstance(payload, dict) and payload.get("action") == "answer" and payload.get("answer"):
        logger.info("ui normalize_answer mode=action_answer")
        return str(payload.get("answer")).strip(), None, None

    if isinstance(payload, dict) and isinstance(payload.get("tools"), list):
        tools = [item for item in payload["tools"] if isinstance(item, dict)]
        base_text = prefix or "Encontrei estas ferramentas disponíveis:"
        logger.info("ui normalize_answer mode=tools_list tools_count=%d", len(tools))
        return base_text, tools, None

    inferred_tools = extract_tools_from_payload(payload)
    if inferred_tools:
        base_text = prefix or "Encontrei estas ferramentas disponíveis:"
        logger.info("ui normalize_answer mode=inferred_tools tools_count=%d", len(inferred_tools))
        return base_text, inferred_tools, None

    text_field = extract_text_fields(payload)
    if text_field:
        nested_prefix, nested_payload = parse_embedded_json(text_field)
        nested_tools = extract_tools_from_payload(nested_payload) if nested_payload is not None else []
        if nested_tools:
            base_text = nested_prefix or prefix or "Encontrei estas ferramentas disponíveis:"
            logger.info("ui normalize_answer mode=text_field_nested_tools tools_count=%d", len(nested_tools))
            return base_text, nested_tools, None
        logger.info("ui normalize_answer mode=text_field")
        return text_field, None, None

    nested = find_nested_payload(payload)
    if nested is not None:
        nested_tools = extract_tools_from_payload(nested)
        if nested_tools:
            base_text = prefix or "Encontrei estas ferramentas disponíveis:"
            logger.info("ui normalize_answer mode=nested_tools tools_count=%d", len(nested_tools))
            return base_text, nested_tools, None
        if isinstance(nested, list):
            logger.info("ui normalize_answer mode=nested_list items=%d", len(nested))
            return (prefix or "Resultado retornado em formato de lista:"), None, nested
        if isinstance(nested, dict):
            nested_text = extract_text_fields(nested)
            if nested_text:
                logger.info("ui normalize_answer mode=nested_text")
                return nested_text, None, None
            logger.info("ui normalize_answer mode=nested_dict")
            return (prefix or "Resultado retornado em formato estruturado."), None, [nested]

    if isinstance(payload, list):
        logger.info("ui normalize_answer mode=top_level_list items=%d", len(payload))
        return (prefix or "Resultado retornado em formato de lista:"), None, payload

    if prefix:
        logger.info("ui normalize_answer mode=prefix_only")
        return prefix, None, None
    logger.info("ui normalize_answer mode=fallback_raw")
    return answer.strip(), None, None


def build_history(messages: List[Dict[str, Any]]) -> List[Dict[str, str]]:
    history: List[Dict[str, str]] = []
    for item in messages:
        role = item.get("role")
        content = str(item.get("content", ""))
        if role in {"user", "assistant", "system"}:
            history.append({"role": role, "content": content})
    return history


for message in st.session_state.messages:
    with st.chat_message(message["role"]):
        st.markdown(message["content"])

prompt = st.chat_input("Digite sua mensagem")

if prompt:
    request_id = f"ui_{uuid.uuid4().hex[:8]}"
    started_at = time.monotonic()
    logger.info("ui chat started request_id=%s prompt_len=%d", request_id, len(prompt))
    st.session_state.messages.append({"role": "user", "content": prompt})
    with st.chat_message("user"):
        st.markdown(prompt)

    payload = {
        "message": prompt,
        "history": build_history(st.session_state.messages[:-1]),
    }
    logger.info(
        "ui chat payload ready request_id=%s history_items=%d endpoint=%s",
        request_id,
        len(payload["history"]),
        CHAT_ENDPOINT,
    )

    with st.chat_message("assistant"):
        with st.spinner("Pensando..."):
            try:
                response = requests.post(
                    CHAT_ENDPOINT,
                    json=payload,
                    timeout=(CHATBOT_API_CONNECT_TIMEOUT_SECONDS, CHATBOT_API_READ_TIMEOUT_SECONDS),
                )
                response.raise_for_status()
                data = response.json()
                logger.info(
                    "ui chat api success request_id=%s status_code=%d response_keys=%s",
                    request_id,
                    response.status_code,
                    list(data.keys()),
                )
            except requests.RequestException as exc:
                status_code = getattr(getattr(exc, "response", None), "status_code", None)
                logger.error(
                    "ui chat api error request_id=%s status_code=%s err=%s",
                    request_id,
                    status_code,
                    exc,
                )
                st.error(f"Erro ao chamar chatbot API: {exc}")
                data = {"answer": "Falha ao processar a mensagem."}

        answer = str(data.get("answer", ""))
        clean_answer, parsed_tools, parsed_items = normalize_answer(answer)
        logger.info(
            "ui chat normalized request_id=%s raw_len=%d clean_len=%d tools_detected=%s items_detected=%s",
            request_id,
            len(answer),
            len(clean_answer),
            bool(parsed_tools),
            bool(parsed_items),
        )
        st.markdown(clean_answer)
        if parsed_tools:
            st.caption("Ferramentas detectadas na resposta")
            for item in parsed_tools:
                name = item.get("name", "sem_nome")
                tool_id = item.get("id", "sem_id")
                st.markdown(f"- `{name}` (`{tool_id}`)")
            logger.info("ui chat rendered tools request_id=%s tools_count=%d", request_id, len(parsed_tools))
        if parsed_items:
            st.caption("Dados estruturados detectados")
            st.json(parsed_items)
            logger.info("ui chat rendered items request_id=%s items_count=%d", request_id, len(parsed_items))
        st.session_state.messages.append({"role": "assistant", "content": clean_answer})

        tool_used = data.get("tool_used")
        tool_output = data.get("tool_output")
        st.session_state.last_tool = {
            "tool_used": tool_used,
            "tool_output": tool_output,
        }
        duration_ms = int((time.monotonic() - started_at) * 1000)
        logger.info(
            "ui chat finished request_id=%s duration_ms=%d tool_used=%s",
            request_id,
            duration_ms,
            tool_used,
        )

with st.sidebar:
    if st.button("Limpar conversa"):
        logger.info("ui clear conversation clicked")
        st.session_state.messages = []
        st.session_state.last_tool = None
        st.rerun()

    st.subheader("Ultima tool")
    if st.session_state.last_tool and st.session_state.last_tool.get("tool_used"):
        st.write(f"tool_used: {st.session_state.last_tool['tool_used']}")
        st.json(st.session_state.last_tool.get("tool_output"))
    else:
        st.caption("Nenhuma tool usada ainda.")
