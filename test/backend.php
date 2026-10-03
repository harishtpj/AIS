<?php

echo $_SERVER['REQUEST_METHOD'] . " " . $_SERVER['REQUEST_URI'] . "\n\n";

foreach (getallheaders() as $name => $value) {
    echo "$name: $value\n";
}
