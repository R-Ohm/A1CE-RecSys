# A1CE-RecSys
# How to Run and Test the API

## 1. Prerequisites
Make sure your working directory is the **A1CE_recommender** folder before running any commands.

## 2. Start the Backend
Open **Terminal 1** and run:
```bash
go run .
```

## 3. Start the Test Interface
Open **Terminal 2** and run:
```bash
python3 -m http.server 3000
```

## 4. Access the Test Page
Open a browser and navigate to:
```
http://localhost:3000/test_interface.html
```

## 5. Retrieve Data From A1CE
1. Go to the A1CE website.
2. Right–click and select Inspect to open developer tools.
3. Locate and copy your:
    - ID
    - JWT token
4. Enter these values into the test interface to run API calls.