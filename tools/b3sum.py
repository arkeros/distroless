import blake3
import sys


if __name__ == "__main__":
    for path in sys.argv[1:]:
        with open(path, "rb") as f:
            digest = blake3.blake3(f.read()).hexdigest()
        print(f"{digest}  {path}")
