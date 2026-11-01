from fastapi import Cookie, FastAPI, HTTPException, Response, status
from pydantic import BaseModel
from typing import Optional
import json
import os
import uuid

app = FastAPI(title="AIS session test service")
data_file = "/tmp/items.json"
items = {}
sessions = {}


class LoginRequest(BaseModel):
    username: str
    password: str


class Item(BaseModel):
    name: str
    price: float


def load_items():
    if not os.path.exists(data_file):
        return
    with open(data_file, "r", encoding="utf-8") as file:
        items.update(json.load(file))


def save_items():
    with open(data_file, "w", encoding="utf-8") as file:
        json.dump(items, file)


def require_session(session_token: Optional[str]) -> str:
    if not session_token or session_token not in sessions:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="login required")
    return sessions[session_token]


@app.post("/login")
def login(payload: LoginRequest, response: Response):
    if payload.username != "alice" or payload.password != "password":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid credentials")
    token = uuid.uuid4().hex
    sessions[token] = payload.username
    response.set_cookie("session_token", token, httponly=True)
    return {"username": payload.username}


@app.get("/items")
def list_items(session_token: Optional[str] = Cookie(default=None)):
    require_session(session_token)
    return list(items.values())


@app.post("/items")
def create_item(item: Item, session_token: Optional[str] = Cookie(default=None)):
    require_session(session_token)
    item_id = uuid.uuid4().hex
    record = {"id": item_id, **item.model_dump()}
    items[item_id] = record
    save_items()
    return record


load_items()
