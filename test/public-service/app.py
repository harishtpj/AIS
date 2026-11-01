from fastapi import FastAPI
from pydantic import BaseModel
import json
import os
import uuid

app = FastAPI(title="AIS public test service")
data_file = "/tmp/items.json"
items = {}


class BackendItem(BaseModel):
    itemName: str
    unitPrice: float


def load_items():
    if not os.path.exists(data_file):
        return
    with open(data_file, "r", encoding="utf-8") as file:
        items.update(json.load(file))


def save_items():
    with open(data_file, "w", encoding="utf-8") as file:
        json.dump(items, file)


@app.get("/items", response_model=list[BackendItem])
def list_items():
    return list(items.values())


@app.post("/items", response_model=BackendItem)
def create_item(item: BackendItem):
    item_id = uuid.uuid4().hex
    record = {"itemName": item.itemName, "unitPrice": item.unitPrice}
    items[item_id] = record
    save_items()
    return record


load_items()
