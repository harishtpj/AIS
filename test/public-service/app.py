from fastapi import FastAPI
from pydantic import BaseModel
import json
import os
import uuid

app = FastAPI(title="AIS public test service")
data_file = "/tmp/items.json"
items = {}


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


@app.get("/items")
def list_items():
    return list(items.values())


@app.post("/items")
def create_item(item: Item):
    item_id = uuid.uuid4().hex
    record = {"id": item_id, "itemName": item.name, "unitPrice": item.price}
    items[item_id] = record
    save_items()
    return record


load_items()
